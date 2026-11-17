package config

// Aucun accès par le réseau (1.6.6) : ces tests tiennent que l'adresse d'écoute est une constante, que
// la configuration n'a plus de champ pour la changer, et que ce qui la demandait encore est signalé
// plutôt que suivi.

import (
	"reflect"
	"strings"
	"testing"
)

func TestLeServeurEcouteSurLaBoucleLocaleEtNulleAutrePart(t *testing.T) {
	if ListenHost != "127.0.0.1" {
		t.Fatalf("ListenHost = %q : le serveur ne doit écouter que sur la boucle locale", ListenHost)
	}
	if got := (&Config{Port: "8000"}).ListenAddr(); got != "127.0.0.1:8000" {
		t.Errorf("ListenAddr = %q, attendu 127.0.0.1:8000", got)
	}
}

// Aucune valeur lue dans l'environnement ou dans config.json ne peut changer l'adresse d'écoute.
func TestAucuneVariableNiCleNeChangeLAdresseDEcoute(t *testing.T) {
	writeConfigFile(t, map[string]any{
		"jwt_secret":          "un-secret-suffisamment-long-pour-passer-la-validation",
		"sqlite_path":         "test.db",
		"port":                "8123",
		"host":                "0.0.0.0",
		"tls_cert":            "/x/cert.pem",
		"tls_key":             "/x/key.pem",
		"allow_insecure_http": true,
		"allowed_origins":     "https://compta.exemple.ch",
		"trusted_proxies":     []string{"10.0.0.1"},
	})
	t.Setenv("HOST", "0.0.0.0")
	t.Setenv("TLS_CERT", "/y/cert.pem")
	t.Setenv("ALLOW_INSECURE_HTTP", "true")

	if got := Load().ListenAddr(); got != "127.0.0.1:8123" {
		t.Errorf("ListenAddr = %q : un réglage réseau a été suivi", got)
	}
}

// La configuration n'a plus de champ pour un accès réseau. Si l'un revient, ce test le dit — il se
// rouvre par une décision, pas par inadvertance.
func TestLaConfigurationNePorteAucunReglageReseau(t *testing.T) {
	interdits := []string{"Host", "TLSCert", "TLSKey", "AllowInsecureHTTP", "AllowedOrigins", "TrustedProxies"}
	for _, typ := range []reflect.Type{reflect.TypeOf(Config{}), reflect.TypeOf(fileConfig{})} {
		for _, nom := range interdits {
			if _, ok := typ.FieldByName(nom); ok {
				t.Errorf("%s porte à nouveau le champ %s : l'accès par le réseau a été retiré", typ.Name(), nom)
			}
		}
	}
}

func viderLEnvironnementReseau(t *testing.T) {
	t.Helper()
	for _, k := range []string{"HOST", "TLS_CERT", "TLS_KEY", "TRUSTED_PROXIES", "ALLOW_INSECURE_HTTP", "ALLOWED_ORIGINS"} {
		t.Setenv(k, "")
	}
}

func TestRienEstSignaleQuandLaConfigurationEstLocale(t *testing.T) {
	writeConfigFile(t, map[string]any{
		"jwt_secret": "un-secret-suffisamment-long-pour-passer-la-validation",
		// Ce que l'assistant écrivait : une origine locale. Il n'y a rien à ignorer.
		"allowed_origins": "http://localhost:8000",
		"host":            "127.0.0.1",
	})
	viderLEnvironnementReseau(t)
	t.Setenv("HOST", "127.0.0.1")

	if got := ReglagesReseauIgnores(); len(got) != 0 {
		t.Errorf("signalé à tort : %v", got)
	}
}

func TestLesAnciensReglagesReseauDeConfigJsonSontSignales(t *testing.T) {
	writeConfigFile(t, map[string]any{
		"jwt_secret":          "un-secret-suffisamment-long-pour-passer-la-validation",
		"host":                "0.0.0.0",
		"tls_cert":            "/x/cert.pem",
		"tls_key":             "/x/key.pem",
		"allow_insecure_http": true,
		"trusted_proxies":     []string{"10.0.0.1"},
		"allowed_origins":     "http://localhost:8000, https://compta.exemple.ch",
	})
	viderLEnvironnementReseau(t)

	vu := strings.Join(ReglagesReseauIgnores(), " | ")
	for _, attendu := range []string{"host=0.0.0.0", "tls_cert", "tls_key", "allow_insecure_http", "trusted_proxies", "allowed_origins="} {
		if !strings.Contains(vu, attendu) {
			t.Errorf("%q n'est pas signalé : %s", attendu, vu)
		}
	}
}

func TestLesAnciennesVariablesReseauSontSignaleesSansConfigJson(t *testing.T) {
	// Un dossier de données vide : pas de config.json.
	writeConfigFile(t, map[string]any{})
	viderLEnvironnementReseau(t)
	t.Setenv("HOST", "0.0.0.0")
	t.Setenv("TLS_CERT", "/y/cert.pem")
	t.Setenv("TRUSTED_PROXIES", "10.0.0.1")
	t.Setenv("ALLOW_INSECURE_HTTP", "true")
	t.Setenv("ALLOWED_ORIGINS", "https://compta.exemple.ch")

	vu := strings.Join(ReglagesReseauIgnores(), " | ")
	for _, attendu := range []string{"HOST=0.0.0.0", "TLS_CERT", "TRUSTED_PROXIES", "ALLOW_INSECURE_HTTP=true", "ALLOWED_ORIGINS=https://compta.exemple.ch"} {
		if !strings.Contains(vu, attendu) {
			t.Errorf("%q n'est pas signalé : %s", attendu, vu)
		}
	}
}
