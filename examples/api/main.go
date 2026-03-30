package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jainam-panchal/go-observability/database"
	"github.com/jainam-panchal/go-observability/httpclient"
	"github.com/jainam-panchal/go-observability/logger"
	"github.com/jainam-panchal/go-observability/middleware"
	"github.com/jainam-panchal/go-observability/telemetry"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type user struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

func main() {
	cfg := telemetry.LoadConfigFromEnv()

	shutdown, err := telemetry.Init(cfg)
	if err != nil {
		panic(fmt.Errorf("init telemetry: %w", err))
	}
	defer func() {
		_ = shutdown(context.Background())
	}()

	appLogger, err := logger.New(cfg)
	if err != nil {
		panic(fmt.Errorf("init logger: %w", err))
	}
	zap.ReplaceGlobals(appLogger)

	db, err := gorm.Open(sqlite.Open("file:api-example?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		panic(fmt.Errorf("open gorm db: %w", err))
	}
	db, err = database.InstrumentGORM(db)
	if err != nil {
		panic(fmt.Errorf("instrument gorm db: %w", err))
	}
	if err := db.AutoMigrate(&user{}); err != nil {
		panic(fmt.Errorf("migrate schema: %w", err))
	}
	if err := db.Create(&user{Name: "alice"}).Error; err != nil {
		panic(fmt.Errorf("seed example user: %w", err))
	}

	client := httpclient.NewClient(&http.Client{Timeout: 5 * time.Second})

	router := gin.New()
	router.Use(gin.Recovery())
	middleware.RegisterGinMiddlewares(router)

	router.GET("/users/:id", func(c *gin.Context) {
		ctx := c.Request.Context()

		var out user
		if err := db.WithContext(ctx).First(&out, c.Param("id")).Error; err != nil {
			logger.L(ctx).Error("load user", zap.Error(err))
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com/health", nil)
		if err != nil {
			logger.L(ctx).Error("build downstream request", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "downstream request failed"})
			return
		}

		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}

		logger.L(ctx).Info("served user", zap.Uint("user_id", out.ID))
		c.JSON(http.StatusOK, out)
	})

	if err := router.Run(":8080"); err != nil {
		panic(fmt.Errorf("run api server: %w", err))
	}
}
