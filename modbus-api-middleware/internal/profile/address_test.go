package profile

import (
	"testing"

	"chpp/modbus-api-middleware/internal/domain"
)

func TestResolveModbusAddress(t *testing.T) {
	tests := []struct {
		name, mode                      string
		fc, register, wantFC, wantStart int
	}{
		{"Huawei 3xxxx", "FULL_NOTATION", 4, 32080, 3, 2080},
		{"Huawei 4xxxx", "FULL_NOTATION", 3, 40196, 4, 196},
		{"zero based", "ZERO_BASED", 3, 2080, 3, 2080},
		{"one based", "ONE_BASED", 3, 2081, 3, 2080},
		{"vendor raw", "VENDOR_RAW", 3, 30057, 3, 30057},
		{"raw alias", "RAW", 3, 32080, 3, 32080},
		{"register 30001", "REGISTER_30001", 4, 30057, 4, 56},
		{"register 40001", "REGISTER_40001", 3, 40084, 3, 83},
		{"SMA raw", "SMA", 3, 30769, 3, 30769},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveModbusAddress(tt.mode, domain.RegisterDefinition{Key: "value", FunctionCode: tt.fc, RegisterAddress: tt.register, Length: 2})
			if err != nil || got.FunctionCode != tt.wantFC || int(got.Start) != tt.wantStart || got.Quantity != 2 {
				t.Fatalf("resolved=%+v err=%v", got, err)
			}
		})
	}
}
