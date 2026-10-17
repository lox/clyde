package main

import (
	"testing"

	"github.com/go-mclib/data/pkg/packets"
	ns "github.com/go-mclib/protocol/java_protocol/net_structures"
	"github.com/go-mclib/protocol/nbt"
)

// A configuration registry contains consecutive optional NBT tags, not byte
// arrays. Reading the first tag must leave the next entry intact.
func TestRegistryNBTFraming(t *testing.T) {
	buf := ns.NewWriter()
	_ = buf.WriteIdentifier("minecraft:spike")
	_ = buf.WriteVarInt(2)
	_ = buf.WriteIdentifier("minecraft:first")
	_ = buf.WriteBool(true)
	raw, err := nbt.EncodeNetwork(nbt.Compound{"x": nbt.Int(7)})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = buf.Write(raw)
	_ = buf.WriteIdentifier("minecraft:second")
	_ = buf.WriteBool(false)
	var got packets.S2CRegistryData
	if err := got.Read(ns.NewReader(buf.Bytes())); err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 || got.Entries[0].EntryId != "minecraft:first" || got.Entries[1].EntryId != "minecraft:second" {
		t.Fatalf("entries: %+v", got.Entries)
	}
	compound, ok := got.Entries[0].Data.(nbt.Compound)
	if !ok || compound["x"] != nbt.Int(7) || bool(got.Entries[1].HasData) {
		t.Fatalf("decoded registry: %+v", got)
	}
}
