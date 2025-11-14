package domain

import pb "github.com/xKoRx/echo/sdk/pb/v1"

// AdjustableStops representa offsets configurables de SL/TP con metadata de validación.
type AdjustableStops struct {
	SLOffsetPoints int32
	TPOffsetPoints int32
	StopLevelBreach bool
	Reason string
}

// ToProto convierte la estructura de dominio a proto.
func (a *AdjustableStops) ToProto() *pb.AdjustableStops {
	if a == nil {
		return nil
	}
	return &pb.AdjustableStops{
		SlOffsetPoints: a.SLOffsetPoints,
		TpOffsetPoints: a.TPOffsetPoints,
		StopLevelBreach: a.StopLevelBreach,
		Reason: a.Reason,
	}
}

// AdjustableStopsFromProto crea la estructura de dominio desde proto.
func AdjustableStopsFromProto(proto *pb.AdjustableStops) *AdjustableStops {
	if proto == nil {
		return nil
	}
	return &AdjustableStops{
		SLOffsetPoints: proto.SlOffsetPoints,
		TPOffsetPoints: proto.TpOffsetPoints,
		StopLevelBreach: proto.StopLevelBreach,
		Reason: proto.Reason,
	}
}
