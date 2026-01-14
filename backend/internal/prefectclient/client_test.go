package prefectclient

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestGetDeploymentByName_Integration(t *testing.T) {
	// Skip in CI environment (no Prefect server available)
	if os.Getenv("CI") != "" {
		t.Skip("Skipping integration test in CI environment")
	}

	client := NewClient("http://localhost:4200", 30*time.Second)

	ctx := context.Background()

	// Test health check
	err := client.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
	t.Log("Health check passed")

	// Test get deployment
	deployment, err := client.GetDeploymentByName(ctx, "sync_to_mysql", "sync_to_mysql-deployment")
	if err != nil {
		t.Fatalf("GetDeploymentByName failed: %v", err)
	}

	t.Logf("Deployment found: ID=%s, Name=%s", deployment.ID, deployment.Name)
}
