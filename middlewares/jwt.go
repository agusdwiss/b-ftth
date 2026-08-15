package middlewares

import (
	"os"
	"strings"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// Ganti dengan secret key yang Anda pakai di .env
var JWTSecret = []byte(os.Getenv("JWT_SECRET"))

func Protected() fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")

		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"detail":  "Akses ditolak. Token tidak ditemukan.",
			})
		}

		// Potong tulisan "Bearer " untuk mengambil token aslinya
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		// Parse dan validasi token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fiber.ErrUnauthorized
			}
			return JWTSecret, nil
		})

		if err != nil || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"detail":  "Token tidak valid atau sudah kedaluwarsa.",
			})
		}

		// Ekstrak payload/claims
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"detail":  "Gagal membaca data token.",
			})
		}

		// Simpan ID dan Role ke dalam Locals agar bisa dipakai oleh Controller
		c.Locals("user_id", claims["id"])
		c.Locals("role", claims["role"])

		// Lanjut ke rute berikutnya
		return c.Next()
	}
}