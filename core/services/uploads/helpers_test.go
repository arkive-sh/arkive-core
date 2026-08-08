package uploads

import "testing"

func TestExpectedEncryptedPartSize(t *testing.T) {
	const mib = int64(1024 * 1024)

	size, err := expectedEncryptedPartSize(5*mib, 4*mib, 8*mib, 1)
	if err != nil {
		t.Fatal(err)
	}
	if want := 5*mib + 2*encryptedChunkEnvelopeOverheadBytes; size != want {
		t.Fatalf("part size = %d, want %d", size, want)
	}

	if _, err := expectedEncryptedPartSize(5*mib, 4*mib, 8*mib, 2); err == nil {
		t.Fatal("expected out-of-range part to fail")
	}
}
