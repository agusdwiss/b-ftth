package services

import (
	"fmt"
	"ftth-go-backend/database"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Fungsi pembantu untuk membaca output socket (Telah diperbaiki bug buffer-nya)
func readUntil(conn net.Conn, promptRegex *regexp.Regexp, timeout time.Duration) (string, error) {
	conn.SetReadDeadline(time.Now().Add(timeout))
	var buffer []byte
	tmp := make([]byte, 256)

	for {
		n, err := conn.Read(tmp)
		if n > 0 {
			buffer = append(buffer, tmp[:n]...)

			// Jika OLT memunculkan paginasi, kirim spasi
			if strings.Contains(string(buffer), "--More--") || strings.Contains(string(buffer), "---- More ----") {
				conn.Write([]byte(" \r\n"))
				buffer = []byte(strings.Replace(string(buffer), "--More--", "", -1))
			}

			if promptRegex.Match(buffer) {
				return string(buffer), nil
			}
		}
		if err != nil {
			return string(buffer), err
		}
	}
}

// ==========================================
// FUNGSI 1: REGISTRASI ONT
// ==========================================
func RegisterOnt(olt database.Olt, sn, customerName, Description, pppoeUser, pppoePass string) (map[string]interface{}, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", olt.Host, olt.Port), 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("gagal terhubung ke OLT %s: %v", olt.Name, err)
	}
	defer conn.Close()

	loginPrompt := regexp.MustCompile(`(?i)(login|user|username)\s*:`)
	passPrompt := regexp.MustCompile(`(?i)(password|passwd)\s*:`)
	shellPrompt := regexp.MustCompile(`[#>]\s*$`)

	_, err = readUntil(conn, loginPrompt, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("timeout menunggu login prompt")
	}
	conn.Write([]byte(olt.Username + "\r\n"))

	_, err = readUntil(conn, passPrompt, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("timeout menunggu password prompt")
	}
	conn.Write([]byte(olt.Password + "\r\n"))

	_, err = readUntil(conn, shellPrompt, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("gagal login, kredensial salah")
	}

	conn.Write([]byte("terminal length 0\r\n"))
	readUntil(conn, shellPrompt, 1*time.Second)

	conn.Write([]byte("show gpon onu uncfg\r\n"))
	uncfgOutput, _ := readUntil(conn, shellPrompt, 5*time.Second)

	snPattern := fmt.Sprintf(`(?i)(?:gpon-onu_)?(\d+/\d+/\d+):\d+\s+%s`, sn)
	snRegex := regexp.MustCompile(snPattern)
	match := snRegex.FindStringSubmatch(uncfgOutput)

	if len(match) < 2 {
		return map[string]interface{}{
			"success": false, "error_type": "NOT_FOUND",
			"message": fmt.Sprintf("SN %s tidak terdeteksi di OLT %s.", sn, olt.Name),
		}, nil
	}
	ponInterface := match[1]

	conn.Write([]byte(fmt.Sprintf("show running-config interface gpon-olt_%s\r\n", ponInterface)))
	configOutput, _ := readUntil(conn, shellPrompt, 5*time.Second)

	indexRegex := regexp.MustCompile(`onu\s+(\d+)\s+type`)
	usedIndexesMatch := indexRegex.FindAllStringSubmatch(configOutput, -1)

	usedIndexes := make(map[int]bool)
	for _, m := range usedIndexesMatch {
		idx, _ := strconv.Atoi(m[1])
		usedIndexes[idx] = true
	}

	targetIndex := 1
	for usedIndexes[targetIndex] {
		targetIndex++
	}

	if targetIndex > 128 {
		return map[string]interface{}{
			"success": false, "error_type": "FULL",
			"message": fmt.Sprintf("Port PON gpon-olt_%s Penuh.", ponInterface),
		}, nil
	}

	if olt.TemplateScript == "" {
		return map[string]interface{}{
			"success": false, "error_type": "NO_TEMPLATE",
			"message": "Template Script kosong!",
		}, nil
	}

	onuIndexStr := fmt.Sprintf("%s:%d", ponInterface, targetIndex)
	nameCmd := ""
	descCmd := ""
	safeName := strings.TrimSpace(customerName)
	safeDesc := strings.TrimSpace(Description)
	if safeName != "-" {
		nameCmd = "name " + safeName
	}
	if safeDesc != "-" {
		descCmd = "description " + safeDesc
	}

	finalScript := olt.TemplateScript
	finalScript = strings.ReplaceAll(finalScript, "{name_cmd}", nameCmd)
	finalScript = strings.ReplaceAll(finalScript, "{desc_cmd}", descCmd)
	finalScript = strings.ReplaceAll(finalScript, "{pon_interface}", ponInterface)
	finalScript = strings.ReplaceAll(finalScript, "{target_index}", strconv.Itoa(targetIndex))
	finalScript = strings.ReplaceAll(finalScript, "{sn}", sn)
	finalScript = strings.ReplaceAll(finalScript, "{pppoe_user}", pppoeUser)
	finalScript = strings.ReplaceAll(finalScript, "{pppoe_pass}", pppoePass)

	commands := strings.Split(finalScript, "\n")
	for _, cmd := range commands {
		cleanCmd := strings.TrimSpace(cmd)
		if cleanCmd != "" {
			conn.Write([]byte(cleanCmd + "\r\n"))
			time.Sleep(100 * time.Millisecond)
		}
	}

	newONT := database.Ont{
		OltID:       olt.ID,
		OltName:     olt.Name,
		OnuName:     customerName,
		Description: Description,
		Sn:          sn,
		OnuIndex:    onuIndexStr,
		Status:      "Logging",
	}

	if err := database.DB.Create(&newONT).Error; err != nil {
		return map[string]interface{}{
			"success": false, "error_type": "INSERT_DB_FAILED",
			"message": "Gagal Menambahkan ONT ke Database",
		}, nil
	}

	time.Sleep(1 * time.Second)
	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("ONT %s Berhasil didaftarkan ke OLT [%s]", sn, olt.Name),
		"onu_id":  fmt.Sprintf("%s:%d", ponInterface, targetIndex),
	}, nil
}

// ==========================================
// FUNGSI 2: PENCARIAN ONT UNCONFIGURED (DIPERBARUI)
// ==========================================
// Tambahan: Sekarang mengembalikan Raw Teks (string) sebagai Output ke-2
func GetUncfgOnts(olt database.Olt) ([]map[string]interface{}, string, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", olt.Host, olt.Port), 5*time.Second)
	if err != nil {
		return nil, "", fmt.Errorf("gagal terhubung ke OLT %s", olt.Name)
	}
	defer conn.Close()

	loginPrompt := regexp.MustCompile(`(?i)(login|user|username)\s*:`)
	passPrompt := regexp.MustCompile(`(?i)(password|passwd)\s*:`)
	shellPrompt := regexp.MustCompile(`[#>]\s*$`)

	_, err = readUntil(conn, loginPrompt, 3*time.Second)
	if err != nil {
		return nil, "", fmt.Errorf("timeout login prompt")
	}
	conn.Write([]byte(olt.Username + "\r\n"))

	_, err = readUntil(conn, passPrompt, 3*time.Second)
	if err != nil {
		return nil, "", fmt.Errorf("timeout password prompt")
	}
	conn.Write([]byte(olt.Password + "\r\n"))

	_, err = readUntil(conn, shellPrompt, 3*time.Second)
	if err != nil {
		return nil, "", fmt.Errorf("gagal login ke OLT")
	}

	// 1. MATIKAN PAGINASI (Wajib bagi ZTE agar hasil tidak terpotong)
	conn.Write([]byte("terminal length 0\r\n"))
	readUntil(conn, shellPrompt, 1*time.Second)

	// 2. JALANKAN PERINTAH SHOW UNCFG
	conn.Write([]byte("show gpon onu uncfg\r\n"))
	uncfgOutput, _ := readUntil(conn, shellPrompt, 5*time.Second)

	// 3. REGEX FLEKSIBEL (Toleransi ada/tidaknya tulisan gpon-onu_)
	re := regexp.MustCompile(`(?i)(?:gpon-onu_)?(\d+/\d+/\d+):\d+\s+([A-Za-z0-9\-]+)`)
	matches := re.FindAllStringSubmatch(uncfgOutput, -1)

	var results []map[string]interface{}
	for _, match := range matches {
		results = append(results, map[string]interface{}{
			"olt_id":        olt.ID,
			"olt_name":      olt.Name,
			"pon_interface": match[1],
			"sn":            match[2],
		})
	}

	// Kembalikan data dan teks aslinya
	return results, uncfgOutput, nil
}

func GetOntLiveDetailCLI(olt database.Olt, ponIndex string) (map[string]interface{}, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", olt.Host, olt.Port), 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("koneksi telnet gagal: %v", err)
	}
	defer conn.Close()

	loginPrompt := regexp.MustCompile(`(?i)(login|user|username)\s*:`)
	passPrompt := regexp.MustCompile(`(?i)(password|passwd)\s*:`)
	shellPrompt := regexp.MustCompile(`[#>]\s*$`)

	// Proses Login
	_, err = readUntil(conn, loginPrompt, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("timeout login prompt")
	}
	conn.Write([]byte(olt.Username + "\r\n"))

	_, err = readUntil(conn, passPrompt, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("timeout password prompt")
	}
	conn.Write([]byte(olt.Password + "\r\n"))

	_, err = readUntil(conn, shellPrompt, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("gagal login ke OLT")
	}

	// Matikan paginasi
	conn.Write([]byte("terminal length 0\r\n"))
	readUntil(conn, shellPrompt, 3*time.Second)

	// 1. EKSTRAK IP WAN
	conn.Write([]byte(fmt.Sprintf("show gpon remote-onu wan-ip gpon-onu_%s\r\n", ponIndex)))
	wanOutput, _ := readUntil(conn, shellPrompt, 5*time.Second)

	ipWan := "-"
	// Sesuai teks Anda: "Current IP: 10.120.1.229"
	ipRegex := regexp.MustCompile(`(?i)Current\s+IP\s*:\s*([\d\.]+)`)
	ipMatch := ipRegex.FindStringSubmatch(wanOutput)
	if len(ipMatch) > 1 {
		if ipMatch[1] != "0.0.0.0" {
			ipWan = ipMatch[1]
		}
	}

	// 2. EKSTRAK REDAMAN RX ONU
	conn.Write([]byte(fmt.Sprintf("show pon power attenuation gpon-onu_%s\r\n", ponIndex)))
	powOutput, _ := readUntil(conn, shellPrompt, 4*time.Second)

	var rxPower float64 = 0.0
	// Cari pola "Rx:-8.649(dbm)" atau "Rx :-13.150(dbm)"
	rxRegex := regexp.MustCompile(`(?i)Rx\s*:\s*(-?[\d\.]+)`)
	rxMatches := rxRegex.FindAllStringSubmatch(powOutput, -1)

	// rxMatches biasanya menemukan 2 baris: Index [0] milik OLT(up), Index [1] milik ONU(down)
	if len(rxMatches) >= 2 {
		parsedRx, err := strconv.ParseFloat(rxMatches[1][1], 64)
		if err == nil {
			rxPower = parsedRx
		}
	} else if len(rxMatches) == 1 { // Jaga-jaga jika format berubah
		parsedRx, err := strconv.ParseFloat(rxMatches[0][1], 64)
		if err == nil {
			rxPower = parsedRx
		}
	}

	return map[string]interface{}{
		"rx_power": rxPower,
		"ip_wan":   ipWan,
	}, nil
}
