package controllers

import (
	"ftth-go-backend/database"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// Struct untuk membaca input saat membuat OLT baru
type OltCreateRequest struct {
	Name           string `json:"name"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	OltType        string `json:"olt_type"`
	SnmpPort			 int    `json:"snmp_port"`
	SnmpCommunity  string `json:"snmp_community"`
	TemplateScript string `json:"template_script"`
}

// Struct untuk membaca input saat mengedit OLT (semua field dibuat opsional/pointer)
type OltUpdateRequest struct {
	Name           *string `json:"name"`
	Host           *string `json:"host"`
	Port           *int    `json:"port"`
	Username       *string `json:"username"`
	Password       *string `json:"password"`
	OltType        *string `json:"olt_type"`
	SnmpPort			 *int    `json:"snmp_port"`
	SnmpCommunity  *string `json:"snmp_community"`
	TemplateScript *string `json:"template_script"`
}

// Struct khusus untuk response agar password tidak ikut dikirim ke frontend
type OltSafeResponse struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	OltType        string `json:"olt_type"`
	SnmpPort			 int    `json:"snmp_port"`
	SnmpCommunity  string `json:"snmp_community"`
	TemplateScript string `json:"template_script"`
}

// ==========================================
// 1. GET: MENGAMBIL SEMUA OLT MILIK USER
// ==========================================
func GetOlts(c *fiber.Ctx) error {
	userIDFloat := c.Locals("user_id").(float64)
	userID := uint(userIDFloat)

	var olts []database.Olt
	// Filter berdasarkan user_id pemilik
	if err := database.DB.Where("user_id = ?", userID).Find(&olts).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "detail": err.Error()})
	}

	// Mapping ke safe response (tanpa password)
	var response []OltSafeResponse
	for _, o := range olts {
		response = append(response, OltSafeResponse{
			ID:             o.ID,
			Name:           o.Name,
			Host:           o.Host,
			Port:           o.Port,
			OltType:        o.OltType,
			SnmpPort:			 o.SnmpPort,
			SnmpCommunity:  o.SnmpCommunity,
			TemplateScript: o.TemplateScript,
		})
	}

	return c.JSON(fiber.Map{"success": true, "data": response})
}

// ==========================================
// 2. POST: MENAMBAH OLT BARU
// ==========================================
func CreateOlt(c *fiber.Ctx) error {
	userIDFloat := c.Locals("user_id").(float64)
	userID := uint(userIDFloat)

	req := new(OltCreateRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "detail": "Format JSON tidak valid"})
	}

	// Validasi input wajib
	if req.Name == "" || req.Host == "" || req.Username == "" || req.Password == "" || req.OltType == "" {
		return c.Status(400).JSON(fiber.Map{"success": false, "detail": "Data wajib diisi (name, host, username, password, olt_type)"})
	}

	port := req.Port
	if port == 0 {
		port = 23 // Default port telnet
	}

	newOlt := database.Olt{
		UserID:         userID, // Ikat ke user yang sedang login
		Name:           req.Name,
		Host:           req.Host,
		Port:           port,
		Username:       req.Username,
		Password:       req.Password,
		OltType:        req.OltType,
		SnmpPort:			 	req.SnmpPort,
		SnmpCommunity:  req.SnmpCommunity,
		TemplateScript: req.TemplateScript,
	}

	if err := database.DB.Create(&newOlt).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "detail": err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{"success": true, "message": "OLT berhasil ditambahkan", "olt_id": newOlt.ID})
}

// ==========================================
// 3. PUT: MENGEDIT DATA OLT MILIK USER
// ==========================================
func UpdateOlt(c *fiber.Ctx) error {
	userIDFloat := c.Locals("user_id").(float64)
	userID := uint(userIDFloat)

	// Ambil ID OLT dari parameter URL (/api/olts/:id)
	oltID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "detail": "ID OLT tidak valid"})
	}

	// Cari OLT di database dan pastikan itu miliknya
	var olt database.Olt
	if err := database.DB.Where("id = ? AND user_id = ?", oltID, userID).First(&olt).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "detail": "OLT tidak ditemukan atau Anda tidak memiliki akses"})
	}

	req := new(OltUpdateRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "detail": "Format JSON tidak valid"})
	}

	// Update hanya field yang dikirimkan di JSON (Partial Update)
	if req.Name != nil { olt.Name = *req.Name }
	if req.Host != nil { olt.Host = *req.Host }
	if req.Port != nil { olt.Port = *req.Port }
	if req.Username != nil { olt.Username = *req.Username }
	if req.Password != nil { olt.Password = *req.Password }
	if req.OltType != nil { olt.OltType = *req.OltType }
	if req.SnmpPort != nil { olt.SnmpPort = *req.SnmpPort }
	if req.SnmpCommunity != nil { olt.SnmpCommunity = *req.SnmpCommunity }
	if req.TemplateScript != nil { olt.TemplateScript = *req.TemplateScript }

	// Simpan perubahan ke database
	if err := database.DB.Save(&olt).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "detail": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "Data OLT berhasil diperbarui"})
}

// ==========================================
// 4. DELETE: MENGHAPUS OLT
// ==========================================
func DeleteOlt(c *fiber.Ctx) error {
	userIDFloat := c.Locals("user_id").(float64)
	userID := uint(userIDFloat)

	oltID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "detail": "ID OLT tidak valid"})
	}

	// Hapus OLT dengan filter ganda (id & user_id) untuk mencegah menghapus OLT tenant lain
	result := database.DB.Where("id = ? AND user_id = ?", oltID, userID).Delete(&database.Olt{})
	
	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "detail": result.Error.Error()})
	}

	// Jika tidak ada baris yang terhapus (artinya ID salah atau milik orang lain)
	if result.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"success": false, "detail": "OLT tidak ditemukan atau Anda tidak memiliki akses"})
	}

	return c.JSON(fiber.Map{"success": true, "message": "OLT berhasil dihapus"})
}