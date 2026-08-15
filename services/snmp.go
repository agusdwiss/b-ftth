package services

import (
	"fmt"
	"ftth-go-backend/database"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

// 1. Ekstrak Serial Number (ASCII + Hex)
func parseZteSN(value interface{}) string {
	var b []byte
	switch v := value.(type) {
	case string: b = []byte(v)
	case []byte: b = v
	}

	if len(b) == 8 {
		return fmt.Sprintf("%s%02X%02X%02X%02X", string(b[0:4]), b[4], b[5], b[6], b[7])
	} else if len(b) > 0 {
		return fmt.Sprintf("%X", b)
	}
	return ""
}

// 2. Ekstrak OID Index untuk mendapatkan Key Unik dan PON Index (1/Slot/Port:Onu)
func parseZteOid(pduName string) (mapKey string, ponIndex string) {
	// Contoh PDU: .1.3.6.1.4.1.3902.1012.3.28.1.1.5.268501760.1
	oidParts := strings.Split(strings.Trim(pduName, "."), ".")
	if len(oidParts) < 2 {
		return "", ""
	}

	lastStr := oidParts[len(oidParts)-1]
	secondLastStr := oidParts[len(oidParts)-2]

	lastVal, _ := strconv.ParseInt(lastStr, 10, 64)
	secondLastVal, _ := strconv.ParseInt(secondLastStr, 10, 64)

	var ifIndex, onuId int64

	// Deteksi apakah format index adalah (IfIndex.OnuID)
	if secondLastVal > 100000 {
		ifIndex = secondLastVal
		onuId = lastVal
	} else {
		// Jika hanya 1 bagian (IfIndex) yang mengandung OnuID di byte terakhir
		ifIndex = lastVal
		onuId = lastVal & 0xFF
	}

	// Operasi Bitwise untuk menarik Slot dan Port
	slot := (ifIndex >> 16) & 0xFF
	port := (ifIndex >> 8) & 0xFF

	ponIndex = fmt.Sprintf("1/%d/%d:%d", slot, port, onuId)
	
	// MapKey unik gabungan IfIndex dan OnuID untuk mencegah overwriting data
	mapKey = fmt.Sprintf("%d.%d", ifIndex, onuId)

	return mapKey, ponIndex
}

// 3. Mapping Kode Status ZTE
func parseZteStatus(value interface{}) string {
	val := 0
	switch v := value.(type) {
	case int: val = v
	case uint: val = int(v)
	}

	switch val {
	case 1: return "Logging"
	case 2: return "LOS"
	case 3: return "Sync"
	case 4: return "Working"
	case 5: return "DyingGasp"
	case 6: return "AuthFailed"
	case 7: return "Offline"
	default: return "Unknown"
	}
}

// Fungsi Utama: Tarik SN, Name, dan Status
func GetAllOntsViaSNMP(olt database.Olt) ([]map[string]interface{}, error) {
	target := &gosnmp.GoSNMP{
		Target:    olt.Host,
		Port:      uint16(olt.SnmpPort),
		Community: olt.SnmpCommunity,
		Version:   gosnmp.Version2c,
		Timeout:   time.Duration(10) * time.Second, // Diperpanjang untuk OLT dengan ribuan ONT
		Retries:   3,
	}

	err := target.Connect()
	if err != nil {
		return nil, fmt.Errorf("koneksi SNMP gagal: %v", err)
	}
	defer target.Conn.Close()

	oidZteSN := ".1.3.6.1.4.1.3902.1012.3.28.1.1.5"
	oidZteName := ".1.3.6.1.4.1.3902.1012.3.28.1.1.2"
	oidZteDesc := ".1.3.6.1.4.1.3902.1012.3.28.1.1.3"
	oidZteStatus := ".1.3.6.1.4.1.3902.1012.3.28.2.1.4"

	ontMap := make(map[string]map[string]interface{})

	// WALK 1: Ambil Serial Number (SN)
	target.BulkWalk(oidZteSN, func(pdu gosnmp.SnmpPDU) error {
		mapKey, ponIndex := parseZteOid(pdu.Name)
		sn := parseZteSN(pdu.Value)
		
		if mapKey != "" && sn != "" && sn != "0" {
			if _, exists := ontMap[mapKey]; !exists {
				ontMap[mapKey] = make(map[string]interface{})
			}
			ontMap[mapKey]["sn"] = sn
			ontMap[mapKey]["index"] = ponIndex
		}
		return nil
	})

	// WALK 2: Ambil Nama / Deskripsi ONU
	target.BulkWalk(oidZteName, func(pdu gosnmp.SnmpPDU) error {
		mapKey, _ := parseZteOid(pdu.Name)
		if val, exists := ontMap[mapKey]; exists {
			name := ""
			switch v := pdu.Value.(type) {
			case string: name = v
			case []byte: name = string(v)
			}
			val["name"] = strings.TrimSpace(name)
		}
		return nil
	})

	// WALK 3: Ambil Status
	target.BulkWalk(oidZteStatus, func(pdu gosnmp.SnmpPDU) error {
		mapKey, _ := parseZteOid(pdu.Name)
		if val, exists := ontMap[mapKey]; exists {
			val["status"] = parseZteStatus(pdu.Value)
		}
		return nil
	})

	// WALK 4: Ambil Deskripsi Tambahan (Alamat / Paket)
	target.BulkWalk(oidZteDesc, func(pdu gosnmp.SnmpPDU) error {
		mapKey, _ := parseZteOid(pdu.Name)
		if val, exists := ontMap[mapKey]; exists {
			desc := ""
			switch v := pdu.Value.(type) {
			case string: desc = v
			case []byte: desc = string(v)
			}
			val["description"] = strings.TrimSpace(desc)
		}
		return nil
	})

	var results []map[string]interface{}
	for _, data := range ontMap {
		name := data["name"]
		if name == nil || name == "" {
			name = "-" 
		}

		desc := data["description"]
		if desc == nil || desc == "" {
			desc = "-"
		}
		
		status := data["status"]
		if status == nil {
			status = "Unknown"
		}

		results = append(results, map[string]interface{}{
			"olt_id":     	olt.ID,
			"olt_name":   	olt.Name,
			"sn":         	data["sn"],
			"onu_name":   	name,
			"description": 	desc,
			"onu_index":  	data["index"],
			"status":     	status,
		})
	}

	return results, nil
}

func GetOntLiveDetail(olt database.Olt, sn string) (map[string]interface{}, error) {
	target := &gosnmp.GoSNMP{
		Target:    olt.Host,
		Port:      uint16(olt.SnmpPort),
		Community: olt.SnmpCommunity,
		Version:   gosnmp.Version2c,
		Timeout:   time.Duration(6) * time.Second, // Timeout disesuaikan untuk kestabilan walk
		Retries:   2,
	}

	err := target.Connect()
	if err != nil {
		return nil, fmt.Errorf("koneksi SNMP gagal: %v", err)
	}
	defer target.Conn.Close()

	// 1. Cari raw index (IfIndex.OnuID) berdasarkan SN secara live
	oidZteSN := ".1.3.6.1.4.1.3902.1012.3.28.1.1.5"
	var rawIndex string

	_ = target.BulkWalk(oidZteSN, func(pdu gosnmp.SnmpPDU) error {
		currentSN := parseZteSN(pdu.Value)
		if strings.ToUpper(currentSN) == strings.ToUpper(sn) {
			// Bersihkan dot di depan pdu.Name agar split indeks selalu akurat
			nameClean := strings.TrimPrefix(pdu.Name, ".")
			oidParts := strings.Split(nameClean, ".")
			if len(oidParts) >= 2 {
				// Ambil kombinasi IfIndex dan OnuID (misal: 268501760.1)
				rawIndex = fmt.Sprintf("%s.%s", oidParts[len(oidParts)-2], oidParts[len(oidParts)-1])
			}
			return fmt.Errorf("FOUND") // Hentikan walk lebih cepat jika sudah ketemu
		}
		return nil
	})

	if rawIndex == "" {
		return nil, fmt.Errorf("ONT dengan SN %s tidak ditemukan di perangkat OLT saat ini", sn)
	}

	var rxPowerFloat float64 = 0.0
	var ipWanStr string = "-"

	// 2. AMBIL REDAMAN DENGAN TARGETED WALK (Mengatasi Variasi Suffix & Firmware)
	// Kita siapkan OID cabang .11 dan .12 sebagai antisipasi perbedaan tipe firmware ZTE Anda
	oidRxList := []string{
		fmt.Sprintf(".1.3.6.1.4.1.3902.1012.3.50.11.1.1.2.%s", rawIndex),
		fmt.Sprintf(".1.3.6.1.4.1.3902.1012.3.50.12.1.1.2.%s", rawIndex),
	}

	for _, oidRx := range oidRxList {
		_ = target.Walk(oidRx, func(pdu gosnmp.SnmpPDU) error {
			var rawRx int64
			switch v := pdu.Value.(type) {
			case int: rawRx = int64(v)
			case uint: rawRx = int64(v)
			case int64: rawRx = v
			}

			if rawRx != 0 {
				// Konversi integer OLT menjadi nilai desimal standar dBm
				if rawRx > 1000 || rawRx < -1000 {
					rxPowerFloat = float64(rawRx) / 1000.0
				} else if rawRx > 100 || rawRx < -100 {
					rxPowerFloat = float64(rawRx) / 10.0
				} else {
					rxPowerFloat = float64(rawRx)
				}
				return fmt.Errorf("FOUND") // Stop jika data valid didapatkan
			}
			return nil
		})
		if rxPowerFloat != 0.0 {
			break
		}
	}

	// 3. AMBIL IP WAN DENGAN TARGETED WALK & DECODER BINER (Mengatasi Suffix Profil WAN .1, .2, dst)
	oidIpWanBase := fmt.Sprintf(".1.3.6.1.4.1.3902.1012.3.28.2.3.1.2.%s", rawIndex)
	_ = target.Walk(oidIpWanBase, func(pdu gosnmp.SnmpPDU) error {
		switch v := pdu.Value.(type) {
		case string:
			ipWanStr = v
		case []byte:
			// DETEKSI UTAMA: Jika data dikirim berupa raw biner 4-byte IP Address
			if len(v) == 4 {
				ipWanStr = fmt.Sprintf("%d.%d.%d.%d", v[0], v[1], v[2], v[3])
			} else {
				ipWanStr = string(v)
			}
		}
		
		ipWanStr = strings.TrimSpace(ipWanStr)
		// Pastikan string bukan IP kosong default OLT
		if ipWanStr != "" && ipWanStr != "0.0.0.0" && ipWanStr != "-" {
			return fmt.Errorf("FOUND") // Stop jika IP valid ditemukan
		}
		return nil
	})

	// Normalisasi tampilan akhir jika modem diset Bridge Mode atau belum mendapat IP
	if ipWanStr == "" || ipWanStr == "-" || ipWanStr == "0.0.0.0" {
		ipWanStr = "Tidak Mendapatkan IP WAN"
	}

	return map[string]interface{}{
		"rx_power": rxPowerFloat,
		"ip_wan":   ipWanStr,
	}, nil
}