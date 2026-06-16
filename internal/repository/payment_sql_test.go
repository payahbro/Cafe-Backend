package repository

import (
	"strings"
	"testing"
)

func TestUpdatePaymentAfterWebhookCastsStatusParameter(t *testing.T) {
	if !strings.Contains(updatePaymentAfterWebhook, "$2::public.payment_status") {
		t.Fatalf("UpdatePaymentAfterWebhook must cast status parameter to public.payment_status:\n%s", updatePaymentAfterWebhook)
	}
}
