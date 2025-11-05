package domain

import (
	"testing"

	pb "github.com/xKoRx/echo/sdk/pb/v1"
)

func TestClampLotSize(t *testing.T) {
	tests := []struct {
		name      string
		spec      *pb.VolumeSpec
		lot       float64
		expectLot float64
		wantErr   bool
	}{
		{
			name:      "lot already valid",
			spec:      &pb.VolumeSpec{MinVolume: 0.1, MaxVolume: 10, VolumeStep: 0.1},
			lot:       0.5,
			expectLot: 0.5,
			wantErr:   false,
		},
		{
			name:      "below min clamped",
			spec:      &pb.VolumeSpec{MinVolume: 0.1, MaxVolume: 10, VolumeStep: 0.1},
			lot:       0.05,
			expectLot: 0.1,
			wantErr:   true,
		},
		{
			name:      "above max clamped",
			spec:      &pb.VolumeSpec{MinVolume: 0.1, MaxVolume: 1, VolumeStep: 0.1},
			lot:       2,
			expectLot: 1,
			wantErr:   true,
		},
		{
			name:      "step misalignment",
			spec:      &pb.VolumeSpec{MinVolume: 0.01, MaxVolume: 1, VolumeStep: 0.01},
			lot:       0.015,
			expectLot: 0.02,
			wantErr:   true,
		},
		{
			name:      "invalid spec",
			spec:      &pb.VolumeSpec{MinVolume: 0, MaxVolume: 1, VolumeStep: 0.1},
			lot:       0.5,
			expectLot: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clamped, err := ClampLotSize(tt.spec, tt.lot)
			if clamped != tt.expectLot {
				t.Fatalf("expected lot %.4f, got %.4f", tt.expectLot, clamped)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("expected error=%t, got %v", tt.wantErr, err)
			}
		})
	}
}
