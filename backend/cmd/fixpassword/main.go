package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	// 生成正确的密码哈希
	password := "admin123"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Password: %s\n", password)
	fmt.Printf("Hash: %s\n", string(hash))

	// 验证
	err = bcrypt.CompareHashAndPassword(hash, []byte(password))
	fmt.Printf("Verify result: %v\n", err == nil)

	// 更新数据库
	dsn := "root:123456@tcp(localhost:13306)/clawreef"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec("UPDATE users SET password_hash = ? WHERE username = 'admin'", string(hash))
	if err != nil {
		log.Fatal("Failed to update password:", err)
	}

	fmt.Println("Password updated successfully!")
	fmt.Println("\nYou can now login with:")
	fmt.Println("  Username: admin")
	fmt.Println("  Password: admin123")
}
