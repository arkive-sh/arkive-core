package totpservice

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"image/png"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

func newSetup(username string) ([]byte, SetupResult, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer: "Arkive", AccountName: username, SecretSize: 20,
		Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		return nil, SetupResult{}, err
	}
	setup, err := renderKey(key)
	return []byte(key.Secret()), setup, err
}

func renderSetup(username string, secret []byte) (SetupResult, error) {
	rawSecret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(string(secret))
	if err != nil {
		return SetupResult{}, err
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer: "Arkive", AccountName: username, Secret: rawSecret,
		Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
	})
	clear(rawSecret)
	if err != nil {
		return SetupResult{}, err
	}
	return renderKey(key)
}

func renderKey(key *otp.Key) (SetupResult, error) {
	image, err := key.Image(256, 256)
	if err != nil {
		return SetupResult{}, err
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image); err != nil {
		return SetupResult{}, err
	}
	return SetupResult{Secret: key.Secret(), QRCodeDataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())}, nil
}

func verifyTOTP(secret []byte, code string, now time.Time) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return 0, false
	}
	for _, char := range code {
		if char < '0' || char > '9' {
			return 0, false
		}
	}
	options := totp.ValidateOpts{Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}
	valid, err := totp.ValidateCustom(code, string(secret), now, options)
	if err != nil || !valid {
		return 0, false
	}
	currentStep := now.Unix() / int64(options.Period)
	for _, step := range []int64{currentStep, currentStep - 1, currentStep + 1} {
		if step < 0 {
			continue
		}
		expected, err := totp.GenerateCodeCustom(string(secret), time.Unix(step*int64(options.Period), 0), options)
		if err == nil && subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

func newRecoveryCodes() ([]string, [][]byte, error) {
	codes := make([]string, 10)
	hashes := make([][]byte, 10)
	for i := range codes {
		value := make([]byte, 16)
		if _, err := rand.Read(value); err != nil {
			return nil, nil, err
		}
		raw := strings.ToUpper(hex.EncodeToString(value))
		clear(value)
		codes[i] = raw[:8] + "-" + raw[8:16] + "-" + raw[16:24] + "-" + raw[24:]
		hashes[i] = hashRecoveryCode(codes[i])
	}
	return codes, hashes, nil
}

func hashRecoveryCode(code string) []byte {
	code = strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
	hash := sha256.Sum256([]byte(code))
	return hash[:]
}

func (s *Service) sealSecret(userID string, secret []byte) ([]byte, error) {
	aead, err := s.secretAEAD()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := append([]byte(nil), nonce...)
	return aead.Seal(sealed, nonce, secret, []byte("arkive:totp-secret:v1\n"+userID)), nil
}

func (s *Service) openSecret(userID string, sealed []byte) ([]byte, error) {
	aead, err := s.secretAEAD()
	if err != nil {
		return nil, err
	}
	if len(sealed) < aead.NonceSize()+aead.Overhead() {
		return nil, errors.New("encrypted TOTP secret has invalid size")
	}
	nonce := sealed[:aead.NonceSize()]
	return aead.Open(nil, nonce, sealed[aead.NonceSize():], []byte("arkive:totp-secret:v1\n"+userID))
}

func (s *Service) secretAEAD() (cipher.AEAD, error) {
	key := sha256.Sum256([]byte("arkive:totp-secret-key:v1\x00" + s.opaqueSetup))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
