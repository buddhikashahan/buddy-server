package main

import (
	"context"
	"flag"
	"fmt"
	"log"


	"buddy/server/internal/config"
	"buddy/server/internal/domain"
	platformAuth "buddy/server/internal/platform/auth"
	platformFirestore "buddy/server/internal/platform/firestore"
	"buddy/server/internal/user"
)

func main() {
	email := flag.String("email", "admin@buddyai.com", "Email address for the admin user")
	password := flag.String("password", "AdminPassword123!", "Password for the admin user (min 6 chars)")
	name := flag.String("name", "Super Admin", "Display name")
	role := flag.String("role", "admin", "Role (admin, teacher, student)")
	flag.Parse()

	cfg := config.Load()
	ctx := context.Background()

	if cfg.FirebaseKeyPath == "" {
		log.Fatal("FIREBASE_CREDENTIALS_FILE is not set in .env")
	}

	fmt.Printf("Connecting to Firebase (Project: %s)...\n", cfg.GCPProjectID)
	authClient, err := platformAuth.NewFirebaseAuthClient(ctx, cfg.GCPProjectID, cfg.FirebaseKeyPath)
	if err != nil {
		log.Fatalf("Failed to connect to Firebase Auth: %v", err)
	}

	fsClient, err := platformFirestore.NewClient(ctx, cfg.GCPProjectID, cfg.FirestoreDBID, cfg.FirebaseKeyPath)
	if err != nil {
		log.Fatalf("Failed to connect to Firestore: %v", err)
	}
	defer fsClient.Close()

	userRepo := user.NewFirestoreRepository(fsClient)
	userService := user.NewService(userRepo, authClient)

	fmt.Printf("Creating %s account [%s]...\n", *role, *email)

	if *role == "admin" || *role == "teacher" {
		res, err := userService.CreateStaff(ctx, domain.CreateStaffRequest{
			Email:       *email,
			Password:    *password,
			DisplayName: *name,
			Role:        domain.Role(*role),
			EmployeeID:  "EMP-ROOT-001",
			Designation: "Administrator",
			Department:  "Administration",
			Permissions: []string{"*"},
		})
		if err != nil {
			if err == domain.ErrUserAlreadyExists {
				fmt.Printf("User [%s] already exists in Firebase Auth.\n", *email)
			} else {
				log.Fatalf("Failed to create staff: %v", err)
			}
		} else {
			fmt.Printf("Successfully created %s account in Firebase & Firestore!\nUID: %s\n", *role, res.ID)
		}
	} else {
		res, err := userService.CreateStudent(ctx, domain.CreateStudentRequest{
			Email:              *email,
			Password:           *password,
			DisplayName:        *name,
			RegistrationNumber: "STU-ROOT-001",
			Grade:              "12th Grade",
		})
		if err != nil {
			log.Fatalf("Failed to create student: %v", err)
		}
		fmt.Printf("Successfully created student account in Firebase & Firestore!\nUID: %s\n", res.ID)
	}

	fmt.Println("\n------------------------------------------------------------")
	fmt.Println("Credentials to login:")
	fmt.Printf("Email:    %s\n", *email)
	fmt.Printf("Password: %s\n", *password)
	fmt.Printf("Role:     %s\n", *role)
	fmt.Println("------------------------------------------------------------")
}
