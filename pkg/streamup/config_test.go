package streamup

import (
	"context"
	"testing"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		wantErr     bool
		errContains string
	}{
		{
			name: "Valid minimal config",
			config: Config{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				Bucket:          "test-bucket",
				Key:             "test-key",
				FileSize:        100 * 1024 * 1024, // 100 MB
			},
			wantErr: false,
		},
		{
			name: "Valid R2 config with AccountID",
			config: Config{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				Bucket:          "test-bucket",
				Key:             "test-key",
				FileSize:        100 * 1024 * 1024,
				AccountID:       "test-account-id",
			},
			wantErr: false,
		},
		{
			name: "Valid config with custom endpoint",
			config: Config{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				Bucket:          "test-bucket",
				Key:             "test-key",
				FileSize:        100 * 1024 * 1024,
				Endpoint:        "s3.us-west-2.amazonaws.com",
			},
			wantErr: false,
		},
		{
			name: "Valid config with custom workers and queue",
			config: Config{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				Bucket:          "test-bucket",
				Key:             "test-key",
				FileSize:        100 * 1024 * 1024,
				Workers:         8,
				QueueSize:       20,
			},
			wantErr: false,
		},
		{
			name: "Missing AccessKeyID",
			config: Config{
				SecretAccessKey: "test-secret-key",
				Bucket:          "test-bucket",
				Key:             "test-key",
				FileSize:        100 * 1024 * 1024,
			},
			wantErr:     true,
			errContains: "AccessKeyID",
		},
		{
			name: "Missing SecretAccessKey",
			config: Config{
				AccessKeyID: "test-access-key",
				Bucket:      "test-bucket",
				Key:         "test-key",
				FileSize:    100 * 1024 * 1024,
			},
			wantErr:     true,
			errContains: "SecretAccessKey",
		},
		{
			name: "Missing Bucket",
			config: Config{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				Key:             "test-key",
				FileSize:        100 * 1024 * 1024,
			},
			wantErr:     true,
			errContains: "Bucket",
		},
		{
			name: "Missing Key",
			config: Config{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				Bucket:          "test-bucket",
				FileSize:        100 * 1024 * 1024,
			},
			wantErr:     true,
			errContains: "Key",
		},
		{
			name: "Zero FileSize",
			config: Config{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				Bucket:          "test-bucket",
				Key:             "test-key",
				FileSize:        0,
			},
			wantErr:     true,
			errContains: "FileSize",
		},
		{
			name: "Negative FileSize",
			config: Config{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				Bucket:          "test-bucket",
				Key:             "test-key",
				FileSize:        -1000,
			},
			wantErr:     true,
			errContains: "FileSize",
		},
		{
			name: "FileSize exceeds service limits",
			config: Config{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				Bucket:          "test-bucket",
				Key:             "test-key",
				FileSize:        60 * 1024 * 1024 * 1024 * 1024, // 60 TB (exceeds 50TB limit)
			},
			wantErr:     true,
			errContains: "exceeds service limit",
		},
		{
			name: "Invalid custom service limits",
			config: Config{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				Bucket:          "test-bucket",
				Key:             "test-key",
				FileSize:        100 * 1024 * 1024,
				ServiceLimits: &ServiceLimits{
					MinPartSize: 1 * 1024 * 1024, // 1MB - below S3 minimum
					MaxPartSize: 5 * 1024 * 1024 * 1024,
					MaxParts:    10000,
				},
			},
			wantErr:     true,
			errContains: "5MB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Config.Validate() expected error but got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Config.Validate() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("Config.Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestConfig_Validate_Defaults(t *testing.T) {
	cfg := Config{
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Bucket:          "test-bucket",
		Key:             "test-key",
		FileSize:        100 * 1024 * 1024,
		// Workers, QueueSize, ServiceLimits, Context not set - should get defaults
	}

	err := cfg.Validate()
	if err != nil {
		t.Fatalf("Config.Validate() unexpected error = %v", err)
	}

	// Check defaults were applied
	if cfg.Workers != defaultWorkers {
		t.Errorf("Workers = %d, want default %d", cfg.Workers, defaultWorkers)
	}

	if cfg.QueueSize != defaultQueueSize {
		t.Errorf("QueueSize = %d, want default %d", cfg.QueueSize, defaultQueueSize)
	}

	if cfg.ServiceLimits == nil {
		t.Error("ServiceLimits is nil, expected default S3 limits")
	} else {
		defaultLimits := DefaultS3Limits()
		if *cfg.ServiceLimits != defaultLimits {
			t.Errorf("ServiceLimits = %+v, want %+v", cfg.ServiceLimits, defaultLimits)
		}
	}

	if cfg.Context == nil {
		t.Error("Context is nil, expected background context")
	}
}

func TestConfig_Validate_R2Endpoint(t *testing.T) {
	tests := []struct {
		name         string
		accountID    string
		endpoint     string
		wantEndpoint string
	}{
		{
			name:         "R2 with AccountID, no Endpoint - should auto-generate",
			accountID:    "test-account-123",
			endpoint:     "",
			wantEndpoint: "https://test-account-123.r2.cloudflarestorage.com",
		},
		{
			name:         "R2 with AccountID and custom Endpoint - should keep custom",
			accountID:    "test-account-123",
			endpoint:     "https://custom.endpoint.com",
			wantEndpoint: "https://custom.endpoint.com",
		},
		{
			name:         "No AccountID, no Endpoint - should remain empty",
			accountID:    "",
			endpoint:     "",
			wantEndpoint: "",
		},
		{
			name:         "No AccountID, custom Endpoint - should keep custom",
			accountID:    "",
			endpoint:     "https://s3.amazonaws.com",
			wantEndpoint: "https://s3.amazonaws.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				Bucket:          "test-bucket",
				Key:             "test-key",
				FileSize:        100 * 1024 * 1024,
				AccountID:       tt.accountID,
				Endpoint:        tt.endpoint,
			}

			err := cfg.Validate()
			if err != nil {
				t.Fatalf("Config.Validate() unexpected error = %v", err)
			}

			if cfg.Endpoint != tt.wantEndpoint {
				t.Errorf("Endpoint = %q, want %q", cfg.Endpoint, tt.wantEndpoint)
			}
		})
	}
}

func TestConfig_Validate_Region(t *testing.T) {
	tests := []struct {
		name       string
		accountID  string
		region     string
		wantRegion string
	}{
		{
			name:       "R2 with AccountID, no Region - should default to 'auto'",
			accountID:  "test-account-123",
			region:     "",
			wantRegion: "auto",
		},
		{
			name:       "No AccountID, no Region - should default to 'us-east-1'",
			accountID:  "",
			region:     "",
			wantRegion: "us-east-1",
		},
		{
			name:       "Custom region provided - should keep custom",
			accountID:  "",
			region:     "eu-west-1",
			wantRegion: "eu-west-1",
		},
		{
			name:       "R2 with custom region - should keep custom",
			accountID:  "test-account-123",
			region:     "wnam",
			wantRegion: "wnam",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				Bucket:          "test-bucket",
				Key:             "test-key",
				FileSize:        100 * 1024 * 1024,
				AccountID:       tt.accountID,
				Region:          tt.region,
			}

			err := cfg.Validate()
			if err != nil {
				t.Fatalf("Config.Validate() unexpected error = %v", err)
			}

			if cfg.Region != tt.wantRegion {
				t.Errorf("Region = %q, want %q", cfg.Region, tt.wantRegion)
			}
		})
	}
}

func TestConfig_GetEndpoint(t *testing.T) {
	tests := []struct {
		name         string
		endpoint     string
		region       string
		wantEndpoint string
	}{
		{
			name:         "Custom endpoint provided",
			endpoint:     "https://s3.us-west-2.amazonaws.com",
			region:       "us-west-2",
			wantEndpoint: "https://s3.us-west-2.amazonaws.com",
		},
		{
			name:         "R2 endpoint",
			endpoint:     "https://test-account.r2.cloudflarestorage.com",
			region:       "auto",
			wantEndpoint: "https://test-account.r2.cloudflarestorage.com",
		},
		{
			name:         "No endpoint - should generate AWS S3 endpoint",
			endpoint:     "",
			region:       "us-east-1",
			wantEndpoint: "https://s3.us-east-1.amazonaws.com",
		},
		{
			name:         "No endpoint with eu-west-1",
			endpoint:     "",
			region:       "eu-west-1",
			wantEndpoint: "https://s3.eu-west-1.amazonaws.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Endpoint: tt.endpoint,
				Region:   tt.region,
			}

			gotEndpoint := cfg.GetEndpoint()
			if gotEndpoint != tt.wantEndpoint {
				t.Errorf("GetEndpoint() = %q, want %q", gotEndpoint, tt.wantEndpoint)
			}
		})
	}
}

type contextKey string

func TestConfig_Context(t *testing.T) {
	t.Run("Custom context preserved", func(t *testing.T) {
		customCtx := context.WithValue(context.Background(), contextKey("test"), "value")
		cfg := Config{
			AccessKeyID:     "test-access-key",
			SecretAccessKey: "test-secret-key",
			Bucket:          "test-bucket",
			Key:             "test-key",
			FileSize:        100 * 1024 * 1024,
			Context:         customCtx,
		}

		err := cfg.Validate()
		if err != nil {
			t.Fatalf("Config.Validate() unexpected error = %v", err)
		}

		if cfg.Context != customCtx {
			t.Error("Custom context was not preserved")
		}

		if cfg.Context.Value(contextKey("test")) != "value" {
			t.Error("Context value was not preserved")
		}
	})

	t.Run("Nil context gets default background", func(t *testing.T) {
		cfg := Config{
			AccessKeyID:     "test-access-key",
			SecretAccessKey: "test-secret-key",
			Bucket:          "test-bucket",
			Key:             "test-key",
			FileSize:        100 * 1024 * 1024,
			Context:         nil,
		}

		err := cfg.Validate()
		if err != nil {
			t.Fatalf("Config.Validate() unexpected error = %v", err)
		}

		if cfg.Context == nil {
			t.Error("Context is still nil after validation")
		}
	})
}

func TestConfig_ProgressCallback(t *testing.T) {
	callCount := 0
	var lastBytes int64
	var lastParts int32

	callback := func(bytesUploaded int64, partsUploaded int32) {
		callCount++
		lastBytes = bytesUploaded
		lastParts = partsUploaded
	}

	cfg := Config{
		AccessKeyID:      "test-access-key",
		SecretAccessKey:  "test-secret-key",
		Bucket:           "test-bucket",
		Key:              "test-key",
		FileSize:         100 * 1024 * 1024,
		ProgressCallback: callback,
	}

	err := cfg.Validate()
	if err != nil {
		t.Fatalf("Config.Validate() unexpected error = %v", err)
	}

	// Test callback works
	cfg.ProgressCallback(1024*1024, 1)
	cfg.ProgressCallback(2*1024*1024, 2)

	if callCount != 2 {
		t.Errorf("Callback called %d times, want 2", callCount)
	}
	if lastBytes != 2*1024*1024 {
		t.Errorf("Last bytes = %d, want %d", lastBytes, 2*1024*1024)
	}
	if lastParts != 2 {
		t.Errorf("Last parts = %d, want 2", lastParts)
	}
}
