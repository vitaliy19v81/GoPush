package config

import (
	"github.com/joho/godotenv"
	"log"
	"os"
)

var (
	VapidPublicKey      string
	VapidPrivateKey     string
	JwtSecretKey        string
	JwtRefreshSecretKey string
	PhoneSecretKey      string
	AdminUsername       string
	AdminPassword       string
	DsnPostgres         string
	Environment         string
)

// LoadConfig Загружает переменные окружения из .env файла
func LoadConfig() {
	// Загружаем переменные окружения из .env файла
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	// Загружаем переменные окружения

	VapidPublicKey = os.Getenv("VAPID_PUBLIC_KEY")
	VapidPrivateKey = os.Getenv("VAPID_PRIVATE_KEY")
	JwtSecretKey = os.Getenv("JWT_SECRET_KEY")
	JwtRefreshSecretKey = os.Getenv("JWT_REFRESH_SECRET_KEY")
	PhoneSecretKey = os.Getenv("PHONE_SECRET_KEY")
	AdminUsername = os.Getenv("ADMIN_USERNAME")
	AdminPassword = os.Getenv("ADMIN_PASSWORD")

	DsnPostgres = os.Getenv("DSN_POSTGRES")
	Environment = os.Getenv("ENVIRONMENT") // development или production

	// Проверка на наличие обязательных переменных окружения
	requiredVars := []string{VapidPublicKey, VapidPrivateKey, JwtSecretKey, JwtRefreshSecretKey, PhoneSecretKey, AdminUsername, AdminPassword, DsnPostgres, Environment} // JwtSecretKey, JwtRefreshSecretKey, PhoneSecretKey, AdminUsername, AdminPassword,
	for _, v := range requiredVars {
		if v == "" {
			log.Fatalf("Missing required environment variable: %s", v)
		}
	}

	// TODO попробовать это
	//// Проверка на наличие обязательных переменных окружения
	//requiredVars := map[string]string{
	//	"JWT_SECRET_KEY":        JwtSecretKey,
	//	"JWT_REFRESH_SECRET_KEY": JwtRefreshSecretKey,
	//	"PHONE_SECRET_KEY":       PhoneSecretKey,
	//	"ADMIN_USERNAME":         AdminUsername,
	//	"ADMIN_PASSWORD":         AdminPassword,
	//	"DSN_POSTGRES":           DsnPostgres,
	//	"ENVIRONMENT":            Environment,
	//	"VAPID_PUBLIC_KEY":       VapidPublicKey,
	//	"VAPID_PRIVATE_KEY":      VapidPrivateKey,
	//}
	//
	//for key, value := range requiredVars {
	//	if value == "" {
	//		log.Fatalf("Missing required environment variable: %s", key)
	//	}
	//}
}
