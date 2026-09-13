package controllers

import (
	"ftth-go-backend/database"
	"ftth-go-backend/services"
	"strconv"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
)

type RegisterRequest struct {
	OltID        interface{} `json:"olt_id"`
	SN           string      `json:"sn"`
	CustomerName string      `json:"customer_name"`
	Description  string      `json:"description"`
	PppoeUser    string      `json:"pppoe_user"`
	PppoePass    string      `json:"pppoe_pass"`
}

func RegisterONT(c *fiber.Ctx) error {
	userIDFloat := c.Locals("user_id").(float64)
	userID := uint(userIDFloat)

	req := new(RegisterRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "detail": "Format JSON tidak valid"})
	}

	snUpper := strings.ToUpper(req.SN)
	if req.CustomerName == "" {
		req.CustomerName = "-"
	}
	if req.Description == "" {
		req.Description = "-"
	}

	var olts []database.Olt

	if req.OltID == "AUTO" {
		database.DB.Where("user_id = ?", userID).Find(&olts)
	} else {
		oltIdFloat, _ := req.OltID.(float64)
		database.DB.Where("id = ? AND user_id = ?", uint(oltIdFloat), userID).Find(&olts)
	}

	if len(olts) == 0 {
		return c.Status(404).JSON(fiber.Map{"success": false, "detail": "OLT tidak ditemukan / bukan milik Anda"})
	}

	for _, olt := range olts {
		res, err := services.RegisterOnt(olt, snUpper, req.CustomerName, req.Description, req.PppoeUser, req.PppoePass)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "detail": err.Error()})
		}
		if res["success"] == true {
			return c.Status(200).JSON(res)
		}
		if res["error_type"] == "FULL" {
			return c.Status(500).JSON(fiber.Map{"success": false, "detail": res["message"]})
		}
		if res["error_type"] == "INSERT_DB_FAILED" {
			return c.Status(500).JSON(fiber.Map{"success": false, "detail": res["message"]})
		}
	}
	return c.Status(404).JSON(fiber.Map{"success": false, "detail": "SN tidak ditemukan di seluruh OLT Anda."})
}

// ==========================================
// GET UNCONFIGURED ONTS (DENGAN FITUR DEBUG LOG)
// ==========================================
func GetUnconfiguredOnts(c *fiber.Ctx) error {
	userIDFloat := c.Locals("user_id").(float64)
	userID := uint(userIDFloat)

	oltIDQuery := c.Query("olt_id", "ALL")

	var olts []database.Olt
	if oltIDQuery == "ALL" || oltIDQuery == "" {
		database.DB.Where("user_id = ?", userID).Find(&olts)
	} else {
		oltID, _ := strconv.Atoi(oltIDQuery)
		database.DB.Where("id = ? AND user_id = ?", oltID, userID).Find(&olts)
	}

	if len(olts) == 0 {
		return c.Status(404).JSON(fiber.Map{"success": false, "detail": "OLT tidak ditemukan"})
	}

	var allUncfg []map[string]interface{}
	debugInfo := make(map[string]interface{})

	for _, olt := range olts {
		// Mengambil 3 nilai (onts, rawOutput, error)
		onts, rawOutput, err := services.GetUncfgOnts(olt)

		if err != nil {
			debugInfo[olt.Name] = err.Error()
			continue
		}

		if len(onts) > 0 {
			allUncfg = append(allUncfg, onts...)
		} else {
			// JIKA OLT INI KOSONG, KITA SIMPAN TEKS ASLINYA UNTUK DETEKTIF!
			debugInfo[olt.Name] = rawOutput
		}
	}

	if allUncfg == nil {
		allUncfg = make([]map[string]interface{}, 0)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    allUncfg,
		"debug":   debugInfo, // 👈 Fitur Pelacak Teks OLT
	})
}

func GetAllOntsSnmp(c *fiber.Ctx) error {
	userIDFloat := c.Locals("user_id").(float64)
	userID := uint(userIDFloat)

	oltIDQuery := c.Query("olt_id", "ALL")

	var olts []database.Olt
	if oltIDQuery == "ALL" || oltIDQuery == "" {
		database.DB.Where("user_id = ?", userID).Find(&olts)
	} else {
		oltID, _ := strconv.Atoi(oltIDQuery)
		database.DB.Where("id = ? AND user_id = ?", oltID, userID).Find(&olts)
	}

	if len(olts) == 0 {
		return c.Status(404).JSON(fiber.Map{"success": false, "detail": "OLT tidak ditemukan"})
	}

	var allOnts []map[string]interface{}
	debugErrs := make(map[string]string)

	for _, olt := range olts {
		// Pastikan tidak mengeksekusi OLT yang community string-nya belum diatur
		if olt.SnmpCommunity == "" {
			debugErrs[olt.Name] = "SNMP Community belum disetting di database"
			continue
		}

		onts, err := services.GetAllOntsViaSNMP(olt)
		if err != nil {
			debugErrs[olt.Name] = err.Error()
			continue
		}

		allOnts = append(allOnts, onts...)
	}

	if allOnts == nil {
		allOnts = make([]map[string]interface{}, 0)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    allOnts,
		"total":   len(allOnts),
		"errors":  debugErrs,
	})
}

func GetOntsFromDatabase(c *fiber.Ctx) error {
	// 1. Ambil User ID dari token JWT yang sedang aktif
	userIDFloat := c.Locals("user_id").(float64)
	userID := uint(userIDFloat)

	// 2. PROTEKSI: Cari semua ID OLT yang HANYA milik user ini
	var userOltIDs []uint
	database.DB.Model(&database.Olt{}).Where("user_id = ?", userID).Pluck("id", &userOltIDs)

	// Menerima parameter pagination dari URL query
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	search := c.Query("search", "")

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit

	// Jika user ini belum memiliki OLT sama sekali, langsung kembalikan array kosong
	// agar tidak error saat query "WHERE IN" kosong
	if len(userOltIDs) == 0 {
		return c.JSON(fiber.Map{
			"success": true,
			"data":    []database.Ont{},
			"meta": fiber.Map{
				"page":       page,
				"limit":      limit,
				"total_data": 0,
				"total_page": 0,
			},
		})
	}

	var onts []database.Ont
	var totalData int64

	// 3. Query Database: KUNCI UTAMA KEAMANAN ADA DI SINI
	// Batasi pencarian ONT hanya pada OLT yang ID-nya ada di dalam userOltIDs
	query := database.DB.Model(&database.Ont{}).Where("olt_id IN ?", userOltIDs)

	// Jika ada parameter pencarian (Serial Number atau Nama)
	if search != "" {
		// Menggunakan kurung buka/tutup di GORM untuk kondisi OR yang aman dari kebocoran
		query = query.Where("(sn LIKE ? OR onu_name LIKE ? OR onu_index LIKE ?)", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	// Hitung total data untuk keperluan halaman terakhir di UI
	query.Count(&totalData)

	// Ambil data dengan Limit dan Offset
	query.Order("olt_name ASC, id ASC").Limit(limit).Offset(offset).Find(&onts)

	return c.JSON(fiber.Map{
		"success": true,
		"data":    onts,
		"meta": fiber.Map{
			"page":       page,
			"limit":      limit,
			"total_data": totalData,
			"total_page": (int(totalData) + limit - 1) / limit,
		},
	})
}

func SyncAllOnts(c *fiber.Ctx) error {
	userIDFloat := c.Locals("user_id").(float64)
	userID := uint(userIDFloat)

	var olts []database.Olt
	database.DB.Where("user_id = ?", userID).Find(&olts)

	if len(olts) == 0 {
		return c.Status(404).JSON(fiber.Map{"success": false, "detail": "Tidak ada OLT untuk disinkronisasi"})
	}

	// Gunakan WaitGroup untuk menjalankan SNMP secara paralel
	var wg sync.WaitGroup
	var mu sync.Mutex // Untuk mencegah race condition saat menulis log error

	debugErrs := make(map[string]string)
	totalSynced := 0

	for _, olt := range olts {
		wg.Add(1)

		// Jalankan fungsi dalam Goroutine
		go func(targetOlt database.Olt) {
			defer wg.Done()

			if targetOlt.SnmpCommunity == "" {
				mu.Lock()
				debugErrs[targetOlt.Name] = "SNMP Community belum disetting"
				mu.Unlock()
				return
			}

			// Tarik data via SNMP
			onts, err := services.GetAllOntsViaSNMP(targetOlt)
			if err != nil {
				mu.Lock()
				debugErrs[targetOlt.Name] = err.Error()
				mu.Unlock()
				return
			}

			// --- PROSES UPDATE KE DATABASE MYSQL ---
			// 1. Hapus semua data ONT lama untuk OLT ini (bersihkan tabel)
			database.DB.Where("olt_id = ?", targetOlt.ID).Delete(&database.Ont{})

			// 2. Persiapkan data baru
			var dbOnts []database.Ont
			for _, o := range onts {
				dbOnts = append(dbOnts, database.Ont{
					OltID:       targetOlt.ID,
					OltName:     targetOlt.Name,
					OnuName:     o["onu_name"].(string),
					Description: o["description"].(string),
					Sn:          o["sn"].(string),
					OnuIndex:    o["onu_index"].(string),
					Status:      o["status"].(string),
				})
			}

			if len(dbOnts) > 0 {
				// Insert ke MySQL secara bertahap (500 data per batch) agar anti-gagal
				database.DB.CreateInBatches(&dbOnts, 500)

				mu.Lock()
				totalSynced += len(dbOnts)
				mu.Unlock()
			}

		}(olt) // Parsing copy value agar aman
	}

	// Tunggu sampai SEMUA proses Goroutine selesai
	wg.Wait()

	return c.JSON(fiber.Map{
		"success":      true,
		"message":      "Sinkronisasi selesai",
		"total_synced": totalSynced,
		"errors":       debugErrs,
	})
}

func GetOntLiveDetail(c *fiber.Ctx) error {
	userIDFloat := c.Locals("user_id").(float64)
	userID := uint(userIDFloat)

	oltIDStr := c.Query("olt_id")
	sn := c.Query("sn")

	if oltIDStr == "" || sn == "" {
		return c.Status(400).JSON(fiber.Map{"success": false, "detail": "Parameter olt_id dan sn wajib diisi"})
	}

	oltID, _ := strconv.Atoi(oltIDStr)

	// 1. Ambil data ONT dari database LOKAL untuk mendapatkan pon_index nya (misal: 1/4/1:12)
	var ont database.Ont
	err := database.DB.Where("olt_id = ? AND sn = ?", oltID, sn).First(&ont).Error
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"success": false, "detail": "ONT tidak ditemukan di database. Silakan Sync ulang."})
	}

	// 2. KUNCI KEAMANAN: Pastikan perangkat OLT ini milik user yang sedang login
	var olt database.Olt
	err = database.DB.Where("id = ? AND user_id = ?", oltID, userID).First(&olt).Error
	if err != nil {
		return c.Status(403).JSON(fiber.Map{"success": false, "detail": "Akses ditolak atau OLT tidak valid"})
	}

	// 3. Panggil service CLI Telnet menggunakan OnuIndex yang sudah ada di database
	liveData, err := services.GetOntLiveDetailCLI(olt, ont.OnuIndex)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "detail": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success":  true,
		"rx_power": liveData["rx_power"],
		"ip_wan":   liveData["ip_wan"],
	})
}
