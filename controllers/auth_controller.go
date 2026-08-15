package controllers

import (
	"ftth-go-backend/database"
	"ftth-go-backend/middlewares"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type AuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"` // Opsional saat register
}

// ==========================================
// REGISTER
// ==========================================
func Register(c *fiber.Ctx) error {
	req := new(AuthRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "detail": "Format input salah"})
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "detail": "Gagal mengenkripsi password"})
	}

	role := "user"
	if req.Role == "admin" {
		role = "admin"
	}

	user := database.User{
		Username: req.Username,
		Password: string(hashedPassword),
		Role:     role,
	}

	// Simpan ke DB
	if err := database.DB.Create(&user).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "detail": "Username mungkin sudah terdaftar"})
	}

	return c.Status(201).JSON(fiber.Map{"success": true, "message": "Akun berhasil dibuat"})
}

// ==========================================
// LOGIN
// ==========================================
func Login(c *fiber.Ctx) error {
	req := new(AuthRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "detail": "Format input salah"})
	}

	var user database.User
	database.DB.Where("username = ?", req.Username).First(&user)

	if user.ID == 0 {
		return c.Status(401).JSON(fiber.Map{"success": false, "detail": "Username atau password salah"})
	}

	// Cek Password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return c.Status(401).JSON(fiber.Map{"success": false, "detail": "Username atau password salah"})
	}

	// Buat JWT Token
	claims := jwt.MapClaims{
		"id":   user.ID,
		"sub":  user.Username,
		"role": user.Role,
		"exp":  time.Now().Add(time.Hour * 24).Unix(), // Berlaku 24 Jam
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(middlewares.JWTSecret)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "detail": "Gagal membuat token"})
	}

	return c.JSON(fiber.Map{
		"access_token": tokenString,
		"token_type":   "bearer",
	})
}