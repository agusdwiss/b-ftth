package main

import (
	"ftth-go-backend/controllers"
	"ftth-go-backend/database"
	"ftth-go-backend/middlewares"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/joho/godotenv"
)

func init() {
	godotenv.Load()
}

func main() {
	database.ConnectDB()

	app := fiber.New()
	app.Use(logger.New())
	app.Use(cors.New())

	api := app.Group("/api")

	auth := api.Group("/auth")
	auth.Post("/register", controllers.Register)
	auth.Post("/login", controllers.Login)

	api.Use(middlewares.Protected())

	api.Get("/snmp/onts", controllers.GetAllOntsSnmp)
	api.Get("/unconfigured-onts", controllers.GetUnconfiguredOnts)
	api.Post("/register-ont", controllers.RegisterONT)
	api.Post("/onts/sync", controllers.SyncAllOnts)
	api.Get("/onts/db", controllers.GetOntsFromDatabase)
	api.Get("/onts/detail", controllers.GetOntLiveDetail)
	api.Get("/olts", controllers.GetOlts)
	api.Post("/olts", controllers.CreateOlt)
	api.Put("/olts/:id", controllers.UpdateOlt)
	api.Delete("/olts/:id", controllers.DeleteOlt)

	// Jalankan Server di Port 8000
	app.Listen(":3004")
}
