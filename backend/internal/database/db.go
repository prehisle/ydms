package database

import (
	"fmt"
	"log"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config 数据库配置
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// Connect 连接到数据库并返回 GORM DB 实例
func Connect(cfg Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Info),
		DisableForeignKeyConstraintWhenMigrating: true, // 禁用自动外键创建，我们手动管理
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	log.Println("Database connected successfully")
	return db, nil
}

// AutoMigrate 自动迁移数据库表
func AutoMigrate(db *gorm.DB) error {
	return AutoMigrateWithDefaults(db, AdminDefaults{})
}

// AutoMigrateWithDefaults 自动迁移数据库表并使用指定的默认管理员配置。
func AutoMigrateWithDefaults(db *gorm.DB, defaults AdminDefaults) error {
	log.Println("Running database migrations...")

	// 使用原始的 db（已在 Connect 时配置）迁移所有表
	// 注意：我们在手动创建外键约束，所以不依赖 GORM 自动创建
	err := db.AutoMigrate(&User{}, &CoursePermission{}, &APIKey{}, &DocSyncStatus{}, &WorkflowDefinition{}, &WorkflowRun{}, &WorkflowBatch{}, &SyncBatch{})
	if err != nil {
		return fmt.Errorf("failed to migrate tables: %w", err)
	}

	// 手动创建必要的外键约束
	// User.CreatedBy -> User.ID (自引用)
	err = db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.table_constraints
				WHERE constraint_name = 'fk_users_created_by' AND table_name = 'users'
			) THEN
				ALTER TABLE users ADD CONSTRAINT fk_users_created_by
				FOREIGN KEY (created_by_id) REFERENCES users(id) ON DELETE SET NULL;
			END IF;
		END$$;
	`).Error
	if err != nil {
		log.Printf("Warning: failed to create user self-reference FK: %v", err)
	}

	// APIKey.User -> User.ID
	err = db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.table_constraints
				WHERE constraint_name = 'fk_api_keys_user' AND table_name = 'api_keys'
			) THEN
				ALTER TABLE api_keys ADD CONSTRAINT fk_api_keys_user
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
			END IF;
		END$$;
	`).Error
	if err != nil {
		log.Printf("Warning: failed to create api_keys.user FK: %v", err)
	}

	// APIKey.CreatedBy -> User.ID
	err = db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.table_constraints
				WHERE constraint_name = 'fk_api_keys_created_by' AND table_name = 'api_keys'
			) THEN
				ALTER TABLE api_keys ADD CONSTRAINT fk_api_keys_created_by
				FOREIGN KEY (created_by_id) REFERENCES users(id) ON DELETE SET NULL;
			END IF;
		END$$;
	`).Error
	if err != nil {
		log.Printf("Warning: failed to create api_keys.created_by FK: %v", err)
	}

	// WorkflowRun.CreatedBy -> User.ID
	err = db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.table_constraints
				WHERE constraint_name = 'fk_workflow_runs_created_by' AND table_name = 'workflow_runs'
			) THEN
				ALTER TABLE workflow_runs ADD CONSTRAINT fk_workflow_runs_created_by
				FOREIGN KEY (created_by_id) REFERENCES users(id) ON DELETE SET NULL;
			END IF;
		END$$;
	`).Error
	if err != nil {
		log.Printf("Warning: failed to create workflow_runs.created_by FK: %v", err)
	}

	// WorkflowRun.RetryOf -> WorkflowRun.ID (自引用)
	err = db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.table_constraints
				WHERE constraint_name = 'fk_workflow_runs_retry_of' AND table_name = 'workflow_runs'
			) THEN
				ALTER TABLE workflow_runs ADD CONSTRAINT fk_workflow_runs_retry_of
				FOREIGN KEY (retry_of_id) REFERENCES workflow_runs(id) ON DELETE SET NULL;
			END IF;
		END$$;
	`).Error
	if err != nil {
		log.Printf("Warning: failed to create workflow_runs.retry_of FK: %v", err)
	}

	// WorkflowBatch.CreatedBy -> User.ID
	err = db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.table_constraints
				WHERE constraint_name = 'fk_workflow_batches_created_by' AND table_name = 'workflow_batches'
			) THEN
				ALTER TABLE workflow_batches ADD CONSTRAINT fk_workflow_batches_created_by
				FOREIGN KEY (created_by_id) REFERENCES users(id) ON DELETE SET NULL;
			END IF;
		END$$;
	`).Error
	if err != nil {
		log.Printf("Warning: failed to create workflow_batches.created_by FK: %v", err)
	}

	// SyncBatch.CreatedBy -> User.ID
	err = db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.table_constraints
				WHERE constraint_name = 'fk_sync_batches_created_by' AND table_name = 'sync_batches'
			) THEN
				ALTER TABLE sync_batches ADD CONSTRAINT fk_sync_batches_created_by
				FOREIGN KEY (created_by_id) REFERENCES users(id) ON DELETE SET NULL;
			END IF;
		END$$;
	`).Error
	if err != nil {
		log.Printf("Warning: failed to create sync_batches.created_by FK: %v", err)
	}

	log.Println("Database migrations completed successfully")

	// 创建默认管理员账号（如果不存在）
	if err := ensureDefaultAdmin(db, defaults); err != nil {
		log.Printf("Warning: failed to create default admin: %v", err)
		// 不返回错误，允许应用继续启动
	}

	return nil
}

// AdminDefaults 描述默认管理员账号配置。
type AdminDefaults struct {
	Username    string
	Password    string
	DisplayName string
}

func (d AdminDefaults) WithFallback() AdminDefaults {
	if strings.TrimSpace(d.Username) == "" {
		d.Username = "super_admin"
	}
	if strings.TrimSpace(d.Password) == "" {
		d.Password = "admin123456"
	}
	if strings.TrimSpace(d.DisplayName) == "" {
		d.DisplayName = "超级管理员"
	}
	return d
}

// ensureDefaultAdmin 确保默认管理员账号存在
func ensureDefaultAdmin(db *gorm.DB, defaults AdminDefaults) error {
	defaults = defaults.WithFallback()

	var count int64
	if err := db.Model(&User{}).Where("username = ?", defaults.Username).Count(&count).Error; err != nil {
		return fmt.Errorf("check admin user: %w", err)
	}

	// 如果已经存在指定的超级管理员，跳过
	if count > 0 {
		log.Printf("Super admin user '%s' already exists, skipping default admin creation", defaults.Username)
		return nil
	}

	// 创建默认管理员
	passwordBytes, err := bcrypt.GenerateFromPassword([]byte(defaults.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	defaultAdmin := User{
		Username:     defaults.Username,
		PasswordHash: string(passwordBytes),
		Role:         "super_admin",
		DisplayName:  defaults.DisplayName,
	}

	if err := db.Create(&defaultAdmin).Error; err != nil {
		return fmt.Errorf("create default admin: %w", err)
	}

	log.Println("✓ Default admin created successfully")
	log.Printf("  Username: %s", defaults.Username)
	log.Printf("  Password: %s", defaults.Password)
	log.Println("  ⚠️  WARNING: Please change the default password immediately!")

	return nil
}
