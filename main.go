package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"tecsim-go-server/routes"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/joho/godotenv"
)

var db *sql.DB

func init() {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}

	dsn := os.Getenv("DSN")
	if dsn == "" {
		log.Fatal("DSN not set in .env file")
	}

	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal("Error opening database: ", err)
	}

	// Test the connection
	err = db.Ping()
	if err != nil {
		log.Fatal("Error connecting to database: ", err)
	}
	fmt.Println("Successfully connected to database!")
}

func main() {
	app := fiber.New()
	app.Use(logger.New())

	// Enable CORS
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Using the PassAllAssets route
	app.Post("/api/pass-all-assets", routes.PassAllAssets(db))

	log.Fatal(app.Listen(":3001"))
}
