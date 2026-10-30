package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

// Throwaway local-dev X25519 key matching testdata/backup-v3.json.
const testVectorKeyHex = "807c5fb05b1ea661625cade3c12c5db6059b95d40e91381233d26d5b1574e440"

func loadTestKey(t *testing.T) any {
	t.Helper()
	k, err := LoadPrivateKey(testVectorKeyHex)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func readV3(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/backup-v3.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCalculateKeyFingerprint_X25519(t *testing.T) {
	fp, err := CalculateKeyFingerprint(loadTestKey(t))
	if err != nil {
		t.Fatal(err)
	}
	const want = "1560c7168e948cebfb07a001592a7cc8be5032230abfd9e056b4e7422299ad5f"
	if fp != want {
		t.Fatalf("fingerprint = %s, want %s", fp, want)
	}
}

func TestDecryptBackup_V3TestVector(t *testing.T) {
	raw := readV3(t)
	payload, err := DecryptBackup(raw, loadTestKey(t))
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if payload.TenantID != "plugin_e2e" {
		t.Errorf("tenantId = %q", payload.TenantID)
	}
	if len(payload.Items) != 2 {
		t.Errorf("items = %d, want 2", len(payload.Items))
	}

	var bf BackupFile
	if err := json.Unmarshal(raw, &bf); err != nil {
		t.Fatal(err)
	}
	pub, err := bf.ParsePublicSection()
	if err != nil || pub == nil {
		t.Fatalf("public section: %v %v", pub, err)
	}
	if pub.GeneratedAt == "" || len(pub.Schemas) == 0 {
		t.Errorf("unexpected public section: %+v", pub)
	}
}

func mutate(t *testing.T, raw []byte, fn func(m map[string]any)) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	fn(m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDecryptBackup_V3TamperedPublicDataFails(t *testing.T) {
	raw := mutate(t, readV3(t), func(m map[string]any) {
		m["publicData"] = strings.Replace(m["publicData"].(string), "generatedAt", "generatedAT", 1)
	})
	if _, err := DecryptBackup(raw, loadTestKey(t)); err == nil {
		t.Fatal("expected decryption failure for tampered publicData")
	}
}

func TestDecryptBackup_UnknownVersion(t *testing.T) {
	raw := mutate(t, readV3(t), func(m map[string]any) { m["version"] = "v9" })
	_, err := DecryptBackup(raw, loadTestKey(t))
	if err == nil || !strings.Contains(err.Error(), "unsupported backup version") {
		t.Fatalf("expected unsupported version error, got %v", err)
	}
}

// encryptV2 builds a v2 backup (no AAD) the same way the server does.
func encryptV2(t *testing.T, recipientPriv []byte, plaintext []byte) []byte {
	t.Helper()
	gcmSeal := func(key, iv, pt []byte) []byte {
		blk, err := aes.NewCipher(key)
		if err != nil {
			t.Fatal(err)
		}
		g, err := cipher.NewGCM(blk)
		if err != nil {
			t.Fatal(err)
		}
		return g.Seal(nil, iv, pt, nil)
	}
	rnd := func(n int) []byte {
		b := make([]byte, n)
		if _, err := rand.Read(b); err != nil {
			t.Fatal(err)
		}
		return b
	}

	recipientPub, _ := curve25519.X25519(recipientPriv, curve25519.Basepoint)
	ephPriv := rnd(32)
	ephPub, _ := curve25519.X25519(ephPriv, curve25519.Basepoint)
	shared, _ := curve25519.X25519(ephPriv, recipientPub)
	kek := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, shared, nil, nil), kek); err != nil {
		t.Fatal(err)
	}

	aesKey := rnd(32)
	keyIV := rnd(12)
	encKey := append(append(append([]byte{}, ephPub...), keyIV...), gcmSeal(kek, keyIV, aesKey)...)
	dataIV := rnd(12)
	encData := append(append([]byte{}, dataIV...), gcmSeal(aesKey, dataIV, plaintext)...)

	out, err := json.Marshal(map[string]string{
		"version":        "v2",
		"algorithm":      "X25519-ECIES",
		"keyFingerprint": "unused",
		"encryptedKey":   base64.StdEncoding.EncodeToString(encKey),
		"encryptedData":  base64.StdEncoding.EncodeToString(encData),
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDecryptBackup_V2(t *testing.T) {
	key := loadTestKey(t).([]byte)
	raw := encryptV2(t, key, []byte(`{"tenantId":"t1","syncToken":"s","items":[]}`))
	payload, err := DecryptBackup(raw, key)
	if err != nil {
		t.Fatalf("decrypt v2: %v", err)
	}
	if payload.TenantID != "t1" {
		t.Errorf("tenantId = %q", payload.TenantID)
	}
	var bf BackupFile
	_ = json.Unmarshal(raw, &bf)
	if pub, err := bf.ParsePublicSection(); pub != nil || err != nil {
		t.Errorf("v2 should have no public section, got %v %v", pub, err)
	}
}

func TestDecryptBackup_V3PublicSchemasDecoded(t *testing.T) {
	payload, err := DecryptBackup(readV3(t), loadTestKey(t))
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if len(payload.Schemas) == 0 {
		t.Fatal("no schemas collected from the public section")
	}
	for uri, text := range payload.Schemas {
		if !strings.HasPrefix(text, "{") {
			t.Errorf("schema %s not base64-decoded: %q", uri, text)
		}
	}
}

func TestBackupPayload_LatestFigJSON(t *testing.T) {
	js := `{"syncToken":"s","items":[{"definition":{"key":"k","namespace":"ns","schemaUri":"u","createdAt":"2026-10-30T01:44:45.408Z","updatedAt":"2026-10-30T01:44:45.408Z"},
	"latestFig":{"figId":"f","version":"v1","encrypted":false,"payload":{"data":{"binaryData":"BlVTRCg="},"schema":{"uri":"u","content":"eyJ0eXBlIjoic3RyaW5nIn0="}}}}]}`
	var p BackupPayload
	if err := json.Unmarshal([]byte(js), &p); err != nil {
		t.Fatal(err)
	}
	ff := p.Items[0]
	if len(ff.Figs) != 1 || string(ff.Figs[0].Payload) != "\x06USD(" || ff.Figs[0].Version != "v1" {
		t.Fatalf("unexpected figs: %+v", ff.Figs)
	}
	if ff.DefaultVersion == nil || *ff.DefaultVersion != "v1" {
		t.Fatalf("defaultVersion = %v", ff.DefaultVersion)
	}
	if p.Schemas["u"] != `{"type":"string"}` {
		t.Fatalf("schemas = %v", p.Schemas)
	}
}
