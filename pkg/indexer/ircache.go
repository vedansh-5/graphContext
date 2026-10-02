package indexer

import (
	"bytes"
	"encoding/gob"

	"github.com/vedansh-5/graphcontext/pkg/lang"
)

// irVersionKey is the meta key recording which lang.IRVersion the stored
// graph and cached parses were built with.
const irVersionKey = "ir_version"

func encodeIR(ir *lang.FileIR) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(ir); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeIR(data []byte) (*lang.FileIR, error) {
	ir := &lang.FileIR{}
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(ir); err != nil {
		return nil, err
	}
	// gob drops empty maps; the resolver expects them initialised.
	if ir.Types.Vars == nil {
		ir.Types.Vars = map[string]string{}
	}
	if ir.Types.Fields == nil {
		ir.Types.Fields = map[string]string{}
	}
	if ir.Types.Methods == nil {
		ir.Types.Methods = map[string][]string{}
	}
	if ir.Types.Bases == nil {
		ir.Types.Bases = map[string][]string{}
	}
	return ir, nil
}
