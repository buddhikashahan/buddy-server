# Deploying buddy-server to Cloud Run

## 0. Before anything else: rotate the exposed service-account key

`firebase.json` in this repo (a Google Cloud service-account private key — not to be
confused with the Firebase *deploy config* file of the same name used in the
`buddy-web` repo) was read into an AI assistant conversation during development.
Treat it as exposed:

1. Google Cloud Console → IAM & Admin → Service Accounts → the `firebase-adminsdk`
   account for this project → **Keys** → delete that key. Generate a new one only if
   you still need a local key file for development.
2. **Don't ship a key file in production at all.** Leave `FIREBASE_CREDENTIALS_FILE`
   unset when deploying to Cloud Run — the backend then uses Application Default
   Credentials from the Cloud Run service's own attached identity instead (already how
   `internal/config/config.go` is written; step 3 below grants that identity the
   permissions it needs).

## 1. One-time setup

```bash
gcloud auth login
gcloud config set project buddy-2b92f
gcloud services enable run.googleapis.com artifactregistry.googleapis.com \
  aiplatform.googleapis.com firestore.googleapis.com

gcloud artifacts repositories create buddy \
  --repository-format=docker --location=asia-south1
```

`asia-south1` matches where this project's Firestore database and Storage bucket
already live — keep Cloud Run in the same region to avoid cross-region latency and
egress costs.

## 2. Build, push, and deploy

```bash
gcloud auth configure-docker asia-south1-docker.pkg.dev

docker build -t asia-south1-docker.pkg.dev/buddy-2b92f/buddy/server:latest .
docker push asia-south1-docker.pkg.dev/buddy-2b92f/buddy/server:latest

gcloud run deploy buddy-server \
  --image asia-south1-docker.pkg.dev/buddy-2b92f/buddy/server:latest \
  --region asia-south1 \
  --platform managed \
  --allow-unauthenticated \
  --timeout 3600 \
  --set-env-vars ENV=production,GCP_PROJECT_ID=buddy-2b92f,FIRESTORE_DATABASE_ID=buddy-db,VERTEX_LOCATION=global,GEMINI_MODEL=gemini-3.8-flash,VERTEX_LIVE_MODEL=gemini-live-2.5-flash-native-audio,AUTH_DEV_MODE=false,RATE_LIMIT_RPS=100,RATE_LIMIT_BURST=200,CORS_ALLOWED_ORIGINS=https://buddy-2b92f.web.app
```

If you're using the Cloud Run console's "Continuously deploy from a repository"
option instead (GitHub + Cloud Build) rather than these commands, two settings need
to be set explicitly in that UI, since they're not the console's defaults:

- **Authentication: "Allow public access."** Cloud Run's own IAM layer isn't the
  auth boundary for this API — the app checks every request's Firebase ID token
  itself (see `internal/middleware/auth.go`). "Require authentication" would block
  every ordinary visitor's browser from reaching it at all.
- **Request timeout: raise it from the 300s default to 1800s or the 3600s max.**
  This is a hard ceiling on how long any one connection can stay open, including a
  Live Talk WebSocket session — at the default, Cloud Run disconnects an active
  voice call after 5 minutes regardless of how the app itself behaves.

Note the printed service URL — the `buddy-web` repo's `firebase.json` rewrites
`/api/v1/**` to this service by name (`buddy-server`) and region, not by URL, so no
further wiring is needed there once the service exists under that exact name.

## 3. Grant the runtime identity its permissions

Without this, the service deploys fine but every request fails with a permission
error, since ADC has nothing to authenticate as until this is granted:

```bash
SA="$(gcloud run services describe buddy-server --region asia-south1 --format='value(spec.template.spec.serviceAccountName)')"
for ROLE in roles/datastore.user roles/aiplatform.user roles/firebaseauth.admin; do
  gcloud projects add-iam-policy-binding buddy-2b92f --member="serviceAccount:$SA" --role="$ROLE"
done
```

If that `SA` command returns empty, Cloud Run is using the project's default compute
service account — grant the same roles to
`PROJECT_NUMBER-compute@developer.gserviceaccount.com` instead, or (recommended)
create a dedicated service account first with
`gcloud iam service-accounts create buddy-server-runtime` and redeploy with
`--service-account`.

## 4. Redeploying after code changes

```bash
docker build -t asia-south1-docker.pkg.dev/buddy-2b92f/buddy/server:latest .
docker push asia-south1-docker.pkg.dev/buddy-2b92f/buddy/server:latest
gcloud run deploy buddy-server --image asia-south1-docker.pkg.dev/buddy-2b92f/buddy/server:latest --region asia-south1
```

## Local development

```bash
cp .env.example .env   # then fill in FIREBASE_CREDENTIALS_FILE with a local key, or
                        # leave it blank if you have `gcloud auth application-default
                        # login` set up and want to test against real ADC locally
go run ./cmd/api
```

`go test ./...` passes cleanly with or without local credentials — the three tests
that make live calls to Firebase Auth, Firestore, and Vertex AI (billed, real network
calls) skip automatically when no local service-account key is present, so CI and a
fresh clone both stay green.
