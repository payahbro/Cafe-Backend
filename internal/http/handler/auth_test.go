package handler

import "testing"

func TestValidateRegisterRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     RegisterRequest
		wantErr bool
	}{
		{
			name: "missing fields",
			req: RegisterRequest{
				Email:       "",
				Password:    "",
				FullName:    "",
				PhoneNumber: "",
			},
			wantErr: true,
		},
		{
			name: "invalid phone",
			req: RegisterRequest{
				Email:       "user@example.com",
				Password:    "password",
				FullName:    "User Name",
				PhoneNumber: "08123",
			},
			wantErr: true,
		},
		{
			name: "valid",
			req: RegisterRequest{
				Email:       "user@example.com",
				Password:    "password",
				FullName:    "User Name",
				PhoneNumber: "+628123456789",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := validateRegisterRequest(tt.req)
			if tt.wantErr && len(got) == 0 {
				t.Fatalf("expected validation errors")
			}
			if !tt.wantErr && len(got) > 0 {
				t.Fatalf("did not expect errors, got %v", got)
			}
		})
	}
}
