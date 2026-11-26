package secrets

import (
	"errors"
	"os"
	"testing"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
	"github.com/zalando/go-keyring"
)

func TestMain(m *testing.M) {
	// The tests must never touch the real Keychain.
	keyring.MockInit()
	os.Exit(m.Run())
}

func TestStore(T *testing.T) {
	T.Parallel()

	T.Run("a secret is stored, read back, replaced and removed", func(t *testing.T) {
		t.Parallel()

		store := NewStore(nil, nil)

		_, err := store.Get(t.Context(), "round-trip")
		test.ErrorIs(t, err, ErrNotFound)

		must.NoError(t, store.Set(t.Context(), "round-trip", "first"))
		must.NoError(t, store.Set(t.Context(), "round-trip", "second"))

		secret, err := store.Get(t.Context(), "round-trip")
		must.NoError(t, err)
		test.Eq(t, "second", secret)

		must.NoError(t, store.Delete(t.Context(), "round-trip"))
		test.ErrorIs(t, store.Delete(t.Context(), "round-trip"), ErrNotFound)
	})
}

func TestMemory(T *testing.T) {
	T.Parallel()

	T.Run("it behaves as the real store does", func(t *testing.T) {
		t.Parallel()

		store := &Memory{}

		_, err := store.Get(t.Context(), "x")
		test.ErrorIs(t, err, ErrNotFound)
		test.ErrorIs(t, store.Delete(t.Context(), "x"), ErrNotFound)

		must.NoError(t, store.Set(t.Context(), "x", "secret"))

		secret, err := store.Get(t.Context(), "x")
		must.NoError(t, err)
		test.Eq(t, "secret", secret)

		must.NoError(t, store.Delete(t.Context(), "x"))
	})
}

func TestResolve(T *testing.T) {
	T.Parallel()

	env := func(values map[string]string) func(string) string {
		return func(name string) string { return values[name] }
	}

	T.Run("the environment wins over the store", func(t *testing.T) {
		t.Parallel()

		store := &Memory{Stored: map[string]string{"ld": "stored"}}

		secret, source, err := Resolve(t.Context(), store, env(map[string]string{"TOKEN": "from-env"}), "TOKEN", "ld")
		must.NoError(t, err)
		test.Eq(t, "from-env", secret)
		test.Eq(t, SourceEnvironment, source)
	})

	T.Run("the store answers when the environment has nothing", func(t *testing.T) {
		t.Parallel()

		store := &Memory{Stored: map[string]string{"ld": "stored"}}

		secret, source, err := Resolve(t.Context(), store, env(nil), "TOKEN", "ld")
		must.NoError(t, err)
		test.Eq(t, "stored", secret)
		test.Eq(t, SourceStore, source)
	})

	T.Run("a secret that is nowhere is not an error", func(t *testing.T) {
		t.Parallel()

		secret, source, err := Resolve(t.Context(), &Memory{}, env(nil), "TOKEN", "ld")
		must.NoError(t, err)
		test.Eq(t, "", secret)
		test.Eq(t, "", source)

		secret, source, err = Resolve(t.Context(), nil, nil, "TOKEN", "ld")
		must.NoError(t, err)
		test.Eq(t, "", secret+source)
	})

	T.Run("a store that can't be reached is an error, unless the environment answered", func(t *testing.T) {
		t.Parallel()

		broken := &Memory{Err: errors.New("no secret service")}

		_, _, err := Resolve(t.Context(), broken, env(nil), "TOKEN", "ld")
		test.Error(t, err)

		secret, _, err := Resolve(t.Context(), broken, env(map[string]string{"TOKEN": "from-env"}), "TOKEN", "ld")
		must.NoError(t, err)
		test.Eq(t, "from-env", secret)
	})
}
