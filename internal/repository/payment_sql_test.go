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

func TestListPaymentsCastsStatusParameter(t *testing.T) {
	if !strings.Contains(listPayments, "NULLIF($3, '')::public.payment_status") {
		t.Fatalf("ListPayments must cast status filter to public.payment_status:\n%s", listPayments)
	}
}
