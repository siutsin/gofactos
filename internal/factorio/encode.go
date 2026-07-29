// This file converts blueprints to Factorio's exchange format.
package factorio

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// Encode produces the string players can import into Factorio.
// The format is: version byte "0" + base64(zlib(JSON)).
func Encode(bp *BlueprintWrapper) (string, error) {
	data, err := json.Marshal(bp)
	if err != nil {
		return "", fmt.Errorf("marshalling blueprint JSON: %w", err)
	}

	var buf bytes.Buffer
	zw, err := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	if err != nil {
		return "", fmt.Errorf("creating zlib writer: %w", err)
	}
	_, writeErr := zw.Write(data)
	closeErr := zw.Close()
	if writeErr != nil {
		writeErr = fmt.Errorf("compressing blueprint: %w", writeErr)
	}
	if closeErr != nil {
		closeErr = fmt.Errorf("closing zlib writer: %w", closeErr)
	}
	if err := errors.Join(writeErr, closeErr); err != nil {
		return "", err
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return "0" + encoded, nil
}
