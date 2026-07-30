package delivery

import (
	"path/filepath"
	"testing"
	"time"

	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/store"
)

func TestDeliveryIntervalUsesSavedConfig(t *testing.T) {
	st, err := store.OpenNormalized(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err = st.SaveGatewayConfig(domain.GatewayConfig{SendIntervalSeconds: 17}); err != nil {
		t.Fatal(err)
	}
	if got := (&Worker{Store: st}).Interval(); got != 17*time.Second {
		t.Fatalf("interval=%v", got)
	}
}
