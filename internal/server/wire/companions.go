package wire

import (
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// MountPacket is PutPetToRide / SendPeerCompanionAndVehicle: slot, owner,
// companion ID, and 26 reserved bytes. Peer snapshots use slot one as in C#.
func MountPacket(owner, id uint32, slot byte) []byte {
	return protocol.Builder{protocol.CommandPetControl, protocol.PetControlMount, slot}.U32(owner).U32(id).Bytes(make([]byte, 26))
}

func UnmountPackets(owner uint32) [][]byte {
	return [][]byte{protocol.Builder{protocol.CommandPetControl, protocol.PetControlUnmount}.U32(owner), protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(owner).U8(0)}
}

func PetNamePacket(owner uint32, slot byte, name string) []byte {
	return protocol.Builder{protocol.CommandPetControl, protocol.PetControlBoardVehicleAlternate}.U32(owner).U8(slot).Bytes([]byte(name))
}

// PetMapPacket is CreatePetMapPacket (AC15:4), the pet another player sees beside its owner.
func PetMapPacket(owner, pet uint32, name string) []byte {
	p, _ := protocol.Builder{protocol.CommandPetControl, protocol.PetControlWireCode4}.U32(owner).U32(pet).U8(0).U8(1).String(name)
	return p.U16(0).U16(0).U8(0).U8(0).U16(0)
}

func VehicleMountPacket(c *game.Character) []byte {
	return protocol.Builder{protocol.CommandPetControl, protocol.PetControlVehicleMount, c.VehicleSlot}.U32(c.ID).U16(c.ActiveVehicle)
}

func VehicleDismountPacket(c *game.Character) []byte {
	return protocol.Builder{protocol.CommandPetControl, protocol.PetControlWireCode11, c.VehicleSlot}.U32(c.ID)
}
