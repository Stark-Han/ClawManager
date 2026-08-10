package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	// 连接MySQL（不指定数据库）
	dsn := "root:123456@tcp(localhost:13306)/"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal("Failed to connect to MySQL:", err)
	}
	defer db.Close()

	// 创建数据库
	_, err = db.Exec("CREATE DATABASE IF NOT EXISTS clawreef CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	if err != nil {
		log.Fatal("Failed to create database:", err)
	}
	log.Println("Database 'clawreef' created successfully")

	// 连接到新创建的数据库
	db.Close()
	dsn = "root:123456@tcp(localhost:13306)/clawreef?multiStatements=true"
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal("Failed to connect to clawreef database:", err)
	}
	defer db.Close()

	migrationFiles, err := filepath.Glob("internal/db/migrations/*.sql")
	if err != nil {
		log.Fatal("Failed to list migrations:", err)
	}

	for _, migrationFile := range migrationFiles {
		sqlBytes, readErr := os.ReadFile(migrationFile)
		if readErr != nil {
			log.Fatal("Failed to read SQL file:", readErr)
		}

		scanner := bufio.NewScanner(strings.NewReader(string(sqlBytes)))
		var statement strings.Builder
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(strings.TrimSpace(line), "--") || strings.TrimSpace(line) == "" {
				continue
			}
			statement.WriteString(line)
			statement.WriteString("\n")

			if strings.HasSuffix(strings.TrimSpace(line), ";") {
				sql := statement.String()
				_, err = db.Exec(sql)
				if err != nil {
					log.Printf("Failed to execute statement from %s: %v", migrationFile, err)
					log.Printf("Statement: %s", sql)
				}
				statement.Reset()
			}
		}
	}

	log.Println("Database schema initialized successfully")
	fmt.Println("Admin user created: username='admin', password='admin123'")
}
