package repository

import (
	"strings"
	"testing"
)

func TestGetUserByIdQuerySelectsIsActive(t *testing.T) {
	if !strings.Contains(getUserById, "is_active") {
		t.Fatalf("GetUserById query must select is_active for auth guards")
	}
}
