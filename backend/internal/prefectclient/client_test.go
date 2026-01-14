package prefectclient

import (
	"context"
	"testing"
	"time"
)

func TestGetDeploymentByName_Integration(t *testing.T) {
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
