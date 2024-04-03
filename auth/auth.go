package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"pkbldr/config"
	"strings"
	"time"

	"github.com/maypok86/otter"
	"golang.org/x/crypto/argon2"
)

type Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	KeyLength   uint32
}

var sessionCache otter.Cache[string, string]

func Init() error {
	cache, err := otter.MustBuilder[string, string](100).
		CollectStats().
		Cost(func(key string, value string) uint32 {
			return 1
		}).
		WithTTL(time.Hour).
		Build()
	if err != nil {
		return err
	}
	sessionCache = cache
	return nil
}

func VerifyPassword(password, username string) (bool, error) {
	user, ok := config.Users[username]
	if !ok {
		return false, errors.New("user not found")
	}

	encodedHash := user.PasswordHash
	_, _, _, _, salt, hash, err := decodeHash(encodedHash)
	if err != nil {
		return false, err
	}
	params := &Params{
		Memory:      64 * 1024,
		Iterations:  3,
		Parallelism: 2,
		KeyLength:   32,
	}

	newHash := argon2.IDKey([]byte(password), salt, params.Iterations, params.Memory, params.Parallelism, params.KeyLength)

	if subtle.ConstantTimeCompare(hash, newHash) == 1 {
		return true, nil
	}
	return false, nil
}

func GenerateAndStoreSessionToken(username string) (string, error) {
	token := make([]byte, 32)
	_, err := rand.Read(token)
	if err != nil {
		return "", err
	}

	sessionCache.Set(string(token), username)
	return base64.RawURLEncoding.EncodeToString(token), nil
}

func CheckSessionToken(token string) (bool, string) {
	value, ok := sessionCache.Get(token)
	return ok, value
}

func decodeHash(encodedHash string) (version int, memory, iterations, parallelism uint32, salt, hash []byte, err error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return 0, 0, 0, 0, nil, nil, errors.New("invalid hash format")
	}

	var v int
	_, err = fmt.Sscanf(parts[2], "v=%d", &v)
	if err != nil {
		return 0, 0, 0, 0, nil, nil, err
	}
	version = v

	_, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism)
	if err != nil {
		return 0, 0, 0, 0, nil, nil, err
	}

	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return 0, 0, 0, 0, nil, nil, err
	}

	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return 0, 0, 0, 0, nil, nil, err
	}

	return version, memory, iterations, parallelism, salt, hash, nil
}
