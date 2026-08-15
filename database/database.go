package database

import (
	"log"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type User struct {
	ID         uint   `gorm:"primaryKey"`
	Username   string `gorm:"type:varchar(50);unique;not null"`
	Password   string `gorm:"type:varchar(255);not null"`
	Role       string `gorm:"type:varchar(20);default:'user'"`
	TelegramID string `gorm:"type:varchar(50);unique"`
}

type Olt struct {
	ID             uint   `gorm:"primaryKey"`
	UserID         uint   `gorm:"not null"`
	Name           string `gorm:"type:varchar(100);not null"`
	Host           string `gorm:"type:varchar(15);not null"`
	Port           int    `gorm:"default:23"`
	Username       string `gorm:"type:varchar(50);not null"`
	Password       string `gorm:"type:varchar(50);not null"`
	OltType        string `gorm:"type:varchar(50);default:'ZTE'"`
	TemplateScript string `gorm:"type:text"`
	SnmpPort       int    `gorm:"default:161"`
	SnmpCommunity  string `gorm:"type:varchar(50);default:'public'"`
}

type Ont struct {
	ID        	uint      `gorm:"primaryKey"`
	OltID     	uint      `gorm:"index;not null"`
	OltName   	string    `gorm:"type:varchar(100)"`
	OnuName	 		string    `gorm:"type:varchar(127)"`
	Description	string    `gorm:"type:varchar(200)"`
	Sn        	string    `gorm:"type:varchar(50);index;not null"`
	OnuIndex  	string    `gorm:"type:varchar(50)"`
	Status    	string    `gorm:"type:varchar(20)"`
	UpdatedAt 	time.Time `gorm:"autoUpdateTime"`
}

var DB *gorm.DB

func ConnectDB() {
	dsn := os.Getenv("DB_USER") + ":" + os.Getenv("DB_PASSWORD") + "@tcp(" + os.Getenv("DB_HOST") + ":" + os.Getenv("DB_PORT") + ")/" + os.Getenv("DB_NAME") + "?charset=utf8mb4&parseTime=True&loc=Local"

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Gagal koneksi ke database!\n", err)
	}

	db.AutoMigrate(&User{}, &Olt{}, &Ont{})

	log.Println("Database sukses terhubung!")
	DB = db
}