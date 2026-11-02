package uploads

import "testing"

func TestExpectedEncryptedPartSize(t *testing.T) {
	const mib = int64(1024 * 1024)
	const envelopeOverhead = int64(29)
	if encryptedChunkEnvelopeOverheadBytes != envelopeOverhead {
		t.Fatalf("encrypted envelope overhead = %d, want %d", encryptedChunkEnvelopeOverheadBytes, envelopeOverhead)
	}

	size, err := expectedEncryptedPartSize(5*mib, 4*mib, 8*mib, 1)
	if err != nil {
		t.Fatal(err)
	}
	if want := 5*mib + 2*envelopeOverhead; size != want {
		t.Fatalf("part size = %d, want %d", size, want)
	}

	if _, err := expectedEncryptedPartSize(5*mib, 4*mib, 8*mib, 2); err == nil {
		t.Fatal("expected out-of-range part to fail")
	}
}
