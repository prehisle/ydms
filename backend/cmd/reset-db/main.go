package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/yjxt/ydms/backend/internal/auth"
	"github.com/yjxt/ydms/backend/internal/config"
	"github.com/yjxt/ydms/backend/internal/database"
)

func main() {
	log.Println("=== YDMS 数据库重置工具 ===")

	// 加载 .env 文件
	if err := godotenv.Load(); err != nil {
		log.Println("警告: 未找到 .env 文件，将使用环境变量或默认值")
	}

	// 加载配置
	cfg := config.Load()

	adminDefaults := database.AdminDefaults{
		Username:    cfg.Admin.Username,
		Password:    cfg.Admin.Password,
		DisplayName: cfg.Admin.DisplayName,
	}.WithFallback()

	// 转换为 database.Config
	dbCfg := database.Config{
		Host:     cfg.DB.Host,
		Port:     cfg.DB.Port,
		User:     cfg.DB.User,
		Password: cfg.DB.Password,
		DBName:   cfg.DB.DBName,
		SSLMode:  cfg.DB.SSLMode,
	}

	// 连接数据库
	db, err := database.Connect(dbCfg)
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}

	// 确认操作
	fmt.Println("\n⚠️  警告：此操作将删除所有数据库表和数据！")
	fmt.Println("数据库:", cfg.DB.DBName)
	fmt.Print("是否继续? (输入 'yes' 确认): ")

	var confirm string
	if _, err := fmt.Scanln(&confirm); err != nil {
		log.Fatalf("读取确认输入失败: %v", err)
	}

	if confirm != "yes" {
		log.Println("操作已取消")
		os.Exit(0)
	}

	// 1. 删除所有表
	log.Println("\n步骤 1/3: 删除现有表...")
	err = db.Migrator().DropTable(&database.User{}, &database.CoursePermission{})
	if err != nil {
		log.Fatalf("删除表失败: %v", err)
	}
	log.Println("✓ 表删除成功")

	// 2. 重新创建表
	log.Println("\n步骤 2/3: 重新创建表...")
	err = database.AutoMigrateWithDefaults(db, adminDefaults)
	if err != nil {
		log.Fatalf("创建表失败: %v", err)
	}
	log.Println("✓ 表创建成功")

	// 3. 创建默认超级管理员
	log.Println("\n步骤 3/3: 创建默认超级管理员...")

	passwordHash, err := auth.HashPassword(adminDefaults.Password)
	if err != nil {
		log.Fatalf("密码加密失败: %v", err)
	}

	superAdmin := database.User{
		Username:     adminDefaults.Username,
		PasswordHash: passwordHash,
		Role:         "super_admin",
		DisplayName:  adminDefaults.DisplayName,
	}

	if err := db.Where(database.User{Username: adminDefaults.Username}).
		Assign(superAdmin).
		FirstOrCreate(&superAdmin).Error; err != nil {
		log.Fatalf("创建或更新超级管理员失败: %v", err)
	}

	log.Println("✓ 超级管理员创建成功")

	// 打印登录信息
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("数据库重置完成！")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("\n默认管理员账号:\n")
	fmt.Printf("  用户名: %s\n", adminDefaults.Username)
	fmt.Printf("  密码:   %s\n", adminDefaults.Password)
	fmt.Println("\n⚠️  请在首次登录后立即修改密码！")
	fmt.Println(strings.Repeat("=", 60) + "\n")
}
