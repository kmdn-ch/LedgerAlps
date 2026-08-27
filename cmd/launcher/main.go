// LedgerAlps Launcher — Windows GUI entry point.
//
// Build (no console window):
//
//	GOOS=windows go build -ldflags="-H=windowsgui" -o ledgeralps.exe ./cmd/launcher
//
// Behaviour:
//  1. If %APPDATA%\LedgerAlps\config.json does NOT exist → run setup wizard
//     (serves a local web page, opens browser, collects admin credentials,
//     writes config, starts server, bootstraps first admin).
//  2. If config.json exists → start ledgeralps-server.exe (if not running)
//     and open the app in the default browser.

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/zefix"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// ── Config ────────────────────────────────────────────────────────────────────

type config struct {
	JWTSecret      string `json:"jwt_secret"`
	SQLitePath     string `json:"sqlite_path"`
	Port           string `json:"port"`
	Debug          bool   `json:"debug"`
	AllowedOrigins string `json:"allowed_origins"`
}

func appDataDir() string {
	if runtime.GOOS == "windows" {
		if v := os.Getenv("APPDATA"); v != "" {
			return filepath.Join(v, "LedgerAlps")
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ledgeralps")
}

func configFilePath() string {
	return filepath.Join(appDataDir(), "config.json")
}

func reinstalledMarkerPath() string {
	return filepath.Join(appDataDir(), ".reinstalled")
}

// portValide n'accepte qu'un nombre de port.
//
// La valeur finit à DEUX endroits qui la réinterprètent :
//
//   - `exec.Command("cmd", "/c", "start", "", url)` — Go échappe ses arguments
//     avec syscall.EscapeArg, qui met entre guillemets ce qui contient un
//     espace, mais PAS ce qui contient « & », « | » ou « ^ ». Une valeur sans
//     espace traverse donc intacte jusqu'à cmd.exe, qui la découpe. Vérifié :
//     un port valant `8000&…` fait exécuter ce qui suit l'esperluette ;
//   - `template.JS("\"" + appURL + "\"")` — cette conversion dit à html/template
//     de ne rien échapper. C'est sa raison d'être, et c'est pourquoi ce qu'on y
//     met doit être sûr AVANT.
//
// Échapper deux fois, avec deux règles différentes, serait deux occasions de se
// tromper. Valider une fois à la lecture ferme les deux d'un coup.
func portValide(p string) bool {
	if p == "" || len(p) > 5 {
		return false
	}
	n, err := strconv.Atoi(p)
	return err == nil && n > 0 && n <= 65535
}

func loadConfig() (*config, error) {
	f, err := os.Open(configFilePath())
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var c config
	if err := json.NewDecoder(f).Decode(&c); err != nil {
		return nil, err
	}
	// Un port absent est légitime — l'appelant retombe sur son défaut. Un port
	// présent et illisible ne l'est pas : mieux vaut refuser de démarrer, en le
	// disant, que composer une URL avec.
	if c.Port != "" && !portValide(c.Port) {
		return nil, fmt.Errorf("port invalide dans %s: %q — attendu un nombre de 1 à 65535",
			configFilePath(), c.Port)
	}
	return &c, nil
}

func saveConfig(c *config) error {
	dir := appDataDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.Create(configFilePath())
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(c)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// exeDir returns the directory of the current executable.
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// serverExe returns the path to ledgeralps-server.exe.
func serverExe() string {
	return filepath.Join(exeDir(), "ledgeralps-server.exe")
}

// openBrowser opens the given URL in the system default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// cmd /c start is more reliable than rundll32 in elevated/installer contexts.
		cmd = exec.Command("cmd", "/c", "start", "", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

// waitForServer polls GET /health on the given base URL until it responds 200
// or the context is cancelled.

// Le lanceur ne parle qu'au serveur local, servi en clair : l'écoute réseau
// (et donc TLS) se règle dans l'application, pas ici.

func waitForServer(ctx context.Context, baseURL string, client *http.Client) error {
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				_ = resp.Body.Close()
				return nil
			}
		}
	}
}

// startServer launches ledgeralps-server.exe with the given config as env vars.
// Returns the process; the caller should not wait on it.
func startServer(cfg *config) (*os.Process, error) {
	cmd := exec.Command(serverExe())
	// Seuls les réglages dont le lanceur a lui-même besoin sont transmis. Les
	// options réseau restent dans config.json, que le serveur lit directement :
	// les passer ici en ferait une seconde source de vérité qui, valant sa
	// valeur par défaut, écraserait le fichier — c'est ce qui rendait le
	// réglage TLS impossible à activer.
	cmd.Env = append(os.Environ(),
		"JWT_SECRET="+cfg.JWTSecret,
		"SQLITE_PATH="+cfg.SQLitePath,
		"PORT="+cfg.Port,
		"ALLOWED_ORIGINS="+cfg.AllowedOrigins,
		// Tell the server where it's installed so it can locate the frontend dist folder.
		"LEDGERALPS_INSTALL_DIR="+exeDir(),
	)
	if cfg.Debug {
		cmd.Env = append(cmd.Env, "DEBUG=true")
	}
	// Write server logs to AppData\LedgerAlps\server.log
	logPath := filepath.Join(appDataDir(), "server.log")
	_ = os.MkdirAll(appDataDir(), 0700)
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err == nil {
		cmd.Stdout = lf
		cmd.Stderr = lf
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Process, nil
}

// isServerRunning returns true if the server health endpoint responds.
func isServerRunning(baseURL string, client *http.Client) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// bootstrapPayload is sent to POST /api/v1/auth/bootstrap.
type bootstrapPayload struct {
	Email                string `json:"email"`
	Name                 string `json:"name"`
	Password             string `json:"password"`
	CompanyName          string `json:"company_name,omitempty"`
	LegalForm            string `json:"legal_form,omitempty"`
	AddressStreet        string `json:"address_street,omitempty"`
	AddressPostalCode    string `json:"address_postal_code,omitempty"`
	AddressCity          string `json:"address_city,omitempty"`
	AddressCountry       string `json:"address_country,omitempty"`
	CheNumber            string `json:"che_number,omitempty"`
	VatNumber            string `json:"vat_number,omitempty"`
	IBAN                 string `json:"iban,omitempty"`
	FiscalYearStartMonth int    `json:"fiscal_year_start_month,omitempty"`
}

// bootstrapAdmin calls POST /api/v1/auth/bootstrap to create the first admin + company.
func bootstrapAdmin(baseURL string, payload bootstrapPayload) error {
	if payload.AddressCountry == "" {
		payload.AddressCountry = "CH"
	}
	if payload.FiscalYearStartMonth == 0 {
		payload.FiscalYearStartMonth = 1
	}
	body, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/auth/bootstrap", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return nil // already bootstrapped, not an error
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bootstrap failed (%d): %s", resp.StatusCode, b)
	}
	return nil
}

// ── UID/IDE registry proxy ────────────────────────────────────────────────────

// proxyUIDLookup resolves a CHE number for the setup wizard, which runs before
// the API server and therefore cannot call its endpoint.
//
// The lookup itself lives in internal/core/zefix, shared with the server
// handler. This function used to carry its own copy of that logic, hitting an
// endpoint that answers 403 to everyone; fixing only the server's copy left
// this one broken, which is exactly where first-time users meet it.
func proxyUIDLookup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	raw := strings.TrimSpace(r.URL.Query().Get("che"))
	if raw == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":"paramètre 'che' requis"}`)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	company, err := zefix.Lookup(ctx, raw)
	switch {
	case errors.Is(err, zefix.ErrInvalidFormat):
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":"format IDE invalide — attendu CHE-XXX.XXX.XXX"}`)
	case errors.Is(err, zefix.ErrNotFound):
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"error":"numéro IDE introuvable au registre du commerce"}`)
	case err != nil:
		// Never surface an HTTP status code here: "registre IDE: réponse 403"
		// read like a typing mistake to users who had typed their number
		// perfectly. Say what happened and what they can do instead.
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprint(w, `{"error":"registre IDE momentanément indisponible — saisissez les informations manuellement"}`)
	default:
		out, _ := json.Marshal(company)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
	}
}

// ── Reinstall notification ────────────────────────────────────────────────────

const notifyHTML = `<!DOCTYPE html>
<html lang="fr">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>LedgerAlps — Mise à jour détectée</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    background: #f0f4f8;
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 2rem;
  }
  .card {
    background: #fff;
    border-radius: 12px;
    box-shadow: 0 4px 24px rgba(0,0,0,.10);
    padding: 2.5rem;
    width: 100%;
    max-width: 440px;
    text-align: center;
  }
  .icon { font-size: 2.5rem; margin-bottom: 1rem; }
  h1 { font-size: 1.15rem; font-weight: 700; color: #1a2e4a; margin-bottom: .5rem; }
  p  { font-size: .9rem; color: #64748b; margin-bottom: 1.5rem; line-height: 1.5; }
  .notice {
    background: #eff6ff; border: 1px solid #bfdbfe; color: #1e40af;
    border-radius: 8px; padding: .75rem 1rem; font-size: .85rem;
    margin-bottom: 1.5rem; text-align: left;
  }
  .btn {
    display: inline-block; width: 100%;
    background: #2563eb; color: #fff; text-decoration: none;
    font-size: .92rem; font-weight: 600;
    padding: .8rem 1rem; border-radius: 8px;
    transition: background .15s;
  }
  .btn:hover  { background: #1d4ed8; }
  .btn:focus-visible { outline: 3px solid #93c5fd; outline-offset: 2px; }
</style>
</head>
<body>
<div class="card">
  <div class="icon">&#x2705;</div>
  <h1>LedgerAlps mis à jour</h1>
  <p>L'installation s'est déroulée correctement.</p>
  <div class="notice">
    &#x1F4BE; Configuration existante détectée — vos données comptables ont été conservées.
  </div>
  <a class="btn" href="/ok" autofocus>Ouvrir LedgerAlps</a>
</div>
</body>
</html>`

// runReinstallNotification annonce qu'une mise à jour a eu lieu et que les
// données sont intactes, PUIS ATTEND que l'utilisateur clique.
//
// # Pourquoi l'attente, et pas un compte à rebours
//
// Cet écran portait « Ouverture de LedgerAlps dans 5 secondes… » et se
// remplaçait tout seul. Or c'est le seul moment où le produit dit « vos données
// comptables ont été conservées » — la phrase que quelqu'un qui vient de
// remplacer un logiciel de comptabilité veut lire avant tout. Cinq secondes ne
// suffisent pas à la lire, encore moins à s'en souvenir, et personne ne peut la
// relire ensuite : le message est parti avec la page.
//
// L'écran reste donc jusqu'au clic. C'est aussi ce que fait un installeur
// Windows, qui attend « Terminer ».
//
// # Pourquoi le bouton est un LIEN, sans une ligne de JavaScript
//
// `/ok` ferme l'attente et redirige vers l'application. Le chemin ne dépend
// donc ni d'un script, ni d'une minuterie, ni de l'ordre dans lequel le
// navigateur exécute les choses — il dépend d'une requête HTTP, qui arrive ou
// n'arrive pas.
//
// # Le garde-fou
//
// Si la fenêtre est fermée sans cliquer, plus rien ne viendra : le lanceur
// resterait en mémoire indéfiniment. Une limite le ramasse au bout d'une
// demi-heure — assez longue pour que personne ne la rencontre en revenant
// chercher un café, assez courte pour ne pas laisser un processus orphelin.
//
// Le témoin de réinstallation est effacé DÈS L'ENTRÉE : la fenêtre fermée sans
// clic ne doit pas faire réapparaître cet écran au lancement suivant.
const attenteMaxNotification = 30 * time.Minute

func runReinstallNotification(appURL string) {
	// Delete the sentinel immediately so a normal re-launch won't re-show it.
	_ = os.Remove(reinstalledMarkerPath())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		// If we can't serve the page, skip the notification silently.
		return
	}
	notifyURL := fmt.Sprintf("http://127.0.0.1:%d", ln.Addr().(*net.TCPAddr).Port)
	done := make(chan struct{})
	var uneFois sync.Once
	terminer := func() { uneFois.Do(func() { close(done) }) }

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		type data struct{ AppURL template.JS }
		t, _ := template.New("notify").Parse(notifyHTML)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = t.Execute(w, data{AppURL: template.JS(`"` + appURL + `"`)})
	})

	// Le clic. La redirection part AVANT la fermeture du serveur, sans quoi le
	// navigateur recevrait une connexion coupée au lieu de la page suivante.
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, appURL, http.StatusFound)
		go func() {
			time.Sleep(500 * time.Millisecond)
			terminer()
		}()
	})

	go func() {
		time.Sleep(attenteMaxNotification)
		logInfo("Notification de mise à jour : aucun clic, fermeture du lanceur")
		terminer()
	}()

	srv := &http.Server{Handler: mux}
	go func() {
		<-done
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	go func() {
		time.Sleep(400 * time.Millisecond)
		openBrowser(notifyURL)
	}()

	logInfo("Reinstall notification page at %s", notifyURL)
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		logWarn("notify server: %v", err)
	}
}

// ── Setup wizard ──────────────────────────────────────────────────────────────

const setupHTML = `<!DOCTYPE html>
<html lang="fr">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>LedgerAlps — Configuration initiale</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    background: #f0f4f8;
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 2rem;
  }
  .card {
    background: #fff;
    border-radius: 12px;
    box-shadow: 0 4px 24px rgba(0,0,0,.10);
    padding: 2.5rem 2.5rem 2rem;
    width: 100%;
    max-width: 520px;
  }
  .logo {
    display: flex;
    align-items: center;
    gap: .6rem;
    margin-bottom: 1.6rem;
  }
  .logo svg { width: 36px; height: 36px; flex-shrink: 0; }
  .logo-text { font-size: 1.4rem; font-weight: 700; color: #1a2e4a; letter-spacing: -.5px; }
  .logo-text span { color: #2563eb; }
  h1 { font-size: 1.1rem; font-weight: 600; color: #1a2e4a; margin-bottom: .3rem; }
  .subtitle { font-size: .875rem; color: #64748b; margin-bottom: 1.6rem; }
  .step { display: none; }
  .step.active { display: block; }
  .steps-nav { display: flex; gap: 8px; margin-top: 20px; }
  .steps-nav .btn { margin-top: 0; }
  .btn-ghost {
    background: transparent; color: #475569; border: 1px solid #cbd5e1;
  }
  .progress { display: flex; gap: 6px; margin-bottom: 18px; }
  .progress span {
    flex: 1; height: 3px; border-radius: 2px; background: #e2e8f0;
  }
  .progress span.done { background: #1f4b99; }
  .meter { height: 5px; border-radius: 3px; background: #e2e8f0; overflow: hidden; margin-top: 8px; }
  .meter div { height: 100%; width: 0; transition: width .2s, background .2s; }
  .checks { list-style: none; padding: 0; margin: 8px 0 0; font-size: 12px; }
  .checks li { color: #94a3b8; }
  .checks li.met { color: #15803d; }
  .callout {
    border: 1px solid #cbd5e1; border-radius: 8px; padding: 12px 14px;
    font-size: 13px; line-height: 1.55; margin: 14px 0; background: #f8fafc;
  }
  .callout.warn { border-color: #f59e0b; background: #fffbeb; }
  .choice {
    display: flex; gap: 10px; align-items: flex-start; padding: 12px 14px;
    border: 1px solid #cbd5e1; border-radius: 8px; margin-bottom: 10px; cursor: pointer;
  }
  .choice.selected { border-color: #1f4b99; background: #eff6ff; }
  .choice input { width: auto; margin: 3px 0 0; }
  .choice .t { font-weight: 600; font-size: 14px; }
  .choice .d { font-size: 12.5px; color: #475569; margin-top: 2px; line-height: 1.5; }
  .section-label {
    font-size: .68rem;
    font-weight: 700;
    letter-spacing: .08em;
    text-transform: uppercase;
    color: #2563eb;
    background: #eff6ff;
    border-radius: 5px;
    padding: .3rem .6rem;
    margin: 1.4rem 0 .8rem;
    display: inline-block;
  }
  label { display: block; font-size: .85rem; font-weight: 500; color: #374151; margin-bottom: .25rem; }
  input, select {
    width: 100%;
    padding: .52rem .75rem;
    border: 1.5px solid #e2e8f0;
    border-radius: 7px;
    font-size: .9rem;
    outline: none;
    transition: border-color .15s;
    margin-bottom: .8rem;
    background: #f8fafc;
    color: #1a2e4a;
  }
  input:focus, select:focus { border-color: #2563eb; background: #fff; }
  input::placeholder { color: #b0bec5; }
  .row { display: grid; grid-template-columns: 1fr 1fr; gap: .7rem; }
  .row3 { display: grid; grid-template-columns: 2fr 1fr 2fr; gap: .7rem; }
  .btn {
    width: 100%;
    padding: .75rem;
    background: #2563eb;
    color: #fff;
    border: none;
    border-radius: 8px;
    font-size: .95rem;
    font-weight: 600;
    cursor: pointer;
    margin-top: 1.4rem;
    transition: background .15s;
  }
  .btn:hover { background: #1d4ed8; }
  .btn:disabled { background: #93c5fd; cursor: not-allowed; }
  .error {
    background: #fef2f2; border: 1px solid #fecaca; color: #b91c1c;
    border-radius: 7px; padding: .6rem .8rem; font-size: .85rem;
    margin-bottom: .8rem; display: none;
  }
  .info {
    background: #eff6ff; border: 1px solid #bfdbfe; color: #1e40af;
    border-radius: 7px; padding: .6rem .8rem; font-size: .85rem;
    margin-bottom: .8rem; display: none;
  }
  .spinner {
    display: none; width: 18px; height: 18px;
    border: 2px solid #fff; border-top-color: transparent;
    border-radius: 50%; animation: spin .7s linear infinite; margin: 0 auto;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
  .req { color: #ef4444; margin-left: 2px; }
  .hint { font-size: .78rem; color: #94a3b8; margin-top: -.5rem; margin-bottom: .8rem; }
  .opt { font-size: .75rem; color: #94a3b8; font-weight: 400; }
  .advanced-toggle {
    font-size: .8rem; color: #2563eb; cursor: pointer;
    text-decoration: underline; background: none; border: none;
    padding: 0; margin-top: .4rem; display: block;
  }
  .advanced { display: none; }
</style>
</head>
<body>
<div class="card">
  <div class="logo">
    <svg viewBox="0 0 36 36" fill="none" xmlns="http://www.w3.org/2000/svg">
      <rect width="36" height="36" rx="8" fill="#2563eb"/>
      <path d="M10 26V10h10l6 6v10H10z" fill="none" stroke="#fff" stroke-width="2" stroke-linejoin="round"/>
      <path d="M20 10v6h6" fill="none" stroke="#fff" stroke-width="2" stroke-linejoin="round"/>
      <path d="M14 18h8M14 22h6" stroke="#fff" stroke-width="1.5" stroke-linecap="round"/>
    </svg>
    <span class="logo-text">Ledger<span>Alps</span></span>
  </div>

  <h1>Configuration initiale</h1>
  <p class="subtitle">Bienvenue ! Configurez votre entreprise et créez votre compte administrateur.</p>

  <div class="error" id="errBox"></div>
  <div class="info"  id="infoBox"></div>

  <div class="progress">
    <span id="p1" class="done"></span><span id="p2"></span><span id="p3"></span>
  </div>

  <form id="setupForm">

  <div class="step active" id="step1">

    <!-- ── Entreprise ──────────────────────────────────────────────────── -->
    <div class="section-label">Votre entreprise</div>

    <label for="companyName">Raison sociale <span class="req">*</span></label>
    <input type="text" id="companyName" placeholder="Dupont &amp; Fils Sàrl" autocomplete="organization" required>

    <div class="row">
      <div>
        <label for="legalForm">Forme juridique</label>
        <select id="legalForm">
          <option value="">— choisir —</option>
          <option value="SA">SA</option>
          <option value="Sàrl">Sàrl</option>
          <option value="Association">Association</option>
          <option value="Raison individuelle">Raison individuelle</option>
          <option value="Autre">Autre</option>
        </select>
      </div>
      <div>
        <label for="cheNumber">Numéro IDE <span class="opt">(CHE-xxx.xxx.xxx)</span></label>
        <input type="text" id="cheNumber" placeholder="CHE-123.456.789" autocomplete="off">
        <p class="hint" id="cheHint" style="display:none"></p>
      </div>
    </div>

    <label for="addressStreet">Rue et numéro</label>
    <input type="text" id="addressStreet" placeholder="Route des Alpes 12" autocomplete="street-address">

    <div class="row">
      <div>
        <label for="addressPostalCode">NPA</label>
        <input type="text" id="addressPostalCode" placeholder="1234" autocomplete="postal-code" maxlength="6">
      </div>
      <div>
        <label for="addressCity">Localité</label>
        <input type="text" id="addressCity" placeholder="Lausanne" autocomplete="address-level2">
      </div>
    </div>

    <div class="row">
      <div>
        <label for="vatNumber">N° TVA <span class="opt">(si assujetti)</span></label>
        <input type="text" id="vatNumber" placeholder="CHE-123.456.789 TVA">
      </div>
      <div>
        <label for="iban">IBAN principal <span class="opt">(CH…)</span></label>
        <input type="text" id="iban" placeholder="CH56 0483 5012 3456 7800 9">
      </div>
    </div>

    <!-- ── Compte administrateur ───────────────────────────────────────── -->
    <div class="section-label">Compte administrateur</div>

    <div class="row">
      <div>
        <label for="firstName">Prénom <span class="req">*</span></label>
        <input type="text" id="firstName" placeholder="Jean" autocomplete="given-name" required>
      </div>
      <div>
        <label for="lastName">Nom <span class="req">*</span></label>
        <input type="text" id="lastName" placeholder="Dupont" autocomplete="family-name" required>
      </div>
    </div>

    <label for="email">Adresse e-mail <span class="req">*</span></label>
    <input type="email" id="email" placeholder="admin@entreprise.ch" autocomplete="email" required>

    <label for="password">Mot de passe <span class="req">*</span></label>
    <input type="password" id="password" placeholder="Min. 8 caractères" minlength="8" autocomplete="new-password" required>
    <p class="hint">Minimum 8 caractères. Vous l'utiliserez pour vous connecter à LedgerAlps.</p>

    <!-- ── Paramètres avancés (optionnel) ─────────────────────────────── -->
    <button type="button" class="advanced-toggle" onclick="toggleAdvanced()">&#9660; Paramètres avancés</button>
    <div class="advanced" id="advancedSection">
      <div class="section-label">Serveur</div>
      <label for="port">Port HTTP</label>
      <input type="number" id="port" value="8000" min="1024" max="65535">
    </div>

    <div class="steps-nav">
      <button type="button" class="btn" onclick="goStep(2)">Continuer</button>
    </div>
  </div><!-- /step1 -->

  <!-- ── Étape 2 : protection de la base ────────────────────────── -->
  <div class="step" id="step2">
    <div class="section-label">Protection de la base de données</div>
    <p class="hint" style="margin-top:0">
      Vos livres seront enregistrés dans un fichier sur ce PC. Vous pouvez le chiffrer.
      C'est maintenant que c'est le plus simple : la base est vide, la mise en place est
      instantanée.
    </p>

    <label class="choice" id="choiceOff" onclick="pickEnc(false)">
      <input type="radio" name="enc" id="encOff" checked>
      <span>
        <span class="t">Non, s'en remettre au chiffrement du disque</span>
        <span class="d">Le réglage habituel, et il suffit à la plupart des installations.
        BitLocker ou le Chiffrement de l'appareil, déjà présents dans Windows, protègent
        le poste entier — vos documents et vos courriels compris.</span>
      </span>
    </label>

    <label class="choice" id="choiceOn" onclick="pickEnc(true)">
      <input type="radio" name="enc" id="encOn">
      <span>
        <span class="t">Oui, chiffrer la base</span>
        <span class="d">La protection <b>suit le fichier</b> : copié sur un NAS, un partage
        réseau ou un dossier synchronisé, il reste illisible. C'est la seule chose que
        cela ajoute au chiffrement du disque, mais elle est réelle. Utile surtout si vous
        ne pouvez pas activer BitLocker.</span>
      </span>
    </label>

    <div id="encFields" style="display:none">
      <label for="dbRecovery">Phrase de récupération de la base <span class="req">*</span></label>
      <input type="password" id="dbRecovery" autocomplete="new-password"
             oninput="meter('dbRecovery','dbMeter','dbChecks')">
      <div class="meter"><div id="dbMeter"></div></div>
      <ul class="checks" id="dbChecks"></ul>
      <p class="hint">
        Au quotidien, LedgerAlps s'ouvrira sans rien vous demander : la clé est scellée
        à votre compte Windows. Cette phrase ne sert qu'à rouvrir la base
        <b>depuis un autre ordinateur ou après une réinstallation de Windows</b>.
      </p>
      <div class="callout warn">
        <b>Notez-la maintenant, ailleurs que sur ce PC.</b> Vous n'aurez aucune occasion de
        la revoir, et sans elle une base chiffrée ne se rouvre plus — y compris les dix ans
        de pièces que la loi vous impose de conserver.
      </div>
    </div>

    <div class="steps-nav">
      <button type="button" class="btn btn-ghost" onclick="goStep(1)">Retour</button>
      <button type="button" class="btn" onclick="if (validEnc()) goStep(3)">Continuer</button>
    </div>
  </div><!-- /step2 -->

  <!-- ── Étape 3 : protection des sauvegardes ─────────────────── -->
  <div class="step" id="step3">
    <div class="section-label">Protection des sauvegardes</div>
    <p class="hint" style="margin-top:0">
      LedgerAlps prend une copie complète de votre comptabilité à chaque démarrage.
      C'est la copie qui voyage — un NAS, une clé USB — donc la plus exposée, et c'est
      aussi le seul chemin de retour le jour où ce PC n'est plus là.
    </p>

    <label for="backupPass">Phrase de passe des sauvegardes</label>
    <input type="password" id="backupPass" autocomplete="new-password"
           oninput="meter('backupPass','bkMeter','bkChecks')">
    <div class="meter"><div id="bkMeter"></div></div>
    <ul class="checks" id="bkChecks"></ul>
    <p class="hint">
      Laissez vide pour des sauvegardes en clair — c'est possible, mais alors n'importe qui
      pouvant lire le fichier lit votre comptabilité.
    </p>

    <div class="callout" id="differentWarn" style="display:none">
      <b>Prenez-la différente de la phrase de récupération.</b> Une seule phrase
      compromise ouvrirait sinon les deux — et vous ne les tapez presque jamais, donc
      les retenir toutes les deux ne coûte rien.
    </div>

    <div class="callout warn">
      <b>Notez-la ailleurs que sur ce PC.</b> Sans elle, personne ne peut ouvrir vos
      sauvegardes. Vous non plus.
    </div>

    <div class="steps-nav">
      <button type="button" class="btn btn-ghost" onclick="goStep(2)">Retour</button>
      <button type="submit" class="btn" id="submitBtn">
        <span id="btnText">Démarrer LedgerAlps</span>
        <div class="spinner" id="spinner"></div>
      </button>
    </div>
  </div><!-- /step3 -->
  </form>
</div>

<script>
// ── Navigation par étapes ───────────────────────────────────────
//
// L'étape 1 garde ses champs "required" : passer à la suite sans les remplir
// produirait une erreur du serveur trois écrans plus loin, là où personne ne
// saurait quoi corriger.
var currentStep = 1;

function goStep(n) {
  if (n > 1 && currentStep === 1 && !checkStep1()) return;
  for (var i = 1; i <= 3; i++) {
    document.getElementById('step' + i).className = (i === n) ? 'step active' : 'step';
    document.getElementById('p' + i).className = (i <= n) ? 'done' : '';
  }
  currentStep = n;
  window.scrollTo(0, 0);
}

function checkStep1() {
  var ids = ['companyName', 'firstName', 'lastName', 'email', 'password'];
  for (var i = 0; i < ids.length; i++) {
    var el = document.getElementById(ids[i]);
    if (!el.checkValidity()) { el.reportValidity(); return false; }
  }
  return true;
}

// ── Robustesse de la phrase de passe ────────────────────────────
//
// Même règle que l'application et que le serveur (internal/db/passphrase.go) :
// seize caractères, minuscule, majuscule, chiffre. Reproduite ici parce que
// l'assistant tourne avant que le serveur existe. Le serveur reste l'autorité :
// un écart se solde par son refus au moment de l'enregistrement, pas par une
// phrase faible acceptée en silence.
var MIN_LEN = 16;

function pchecks(p) {
  return [
    [MIN_LEN + ' caract\u00e8res ou plus', Array.from(p).length >= MIN_LEN],
    ['une minuscule', /\p{Ll}/u.test(p)],
    ['une majuscule', /\p{Lu}/u.test(p)],
    ['un chiffre',    /\p{Nd}/u.test(p)]
  ];
}

function pstrong(p) {
  var c = pchecks(p);
  for (var i = 0; i < c.length; i++) if (!c[i][1]) return false;
  return true;
}

function meter(inputId, meterId, checksId) {
  var v = document.getElementById(inputId).value;
  var c = pchecks(v);
  var met = 0;
  for (var i = 0; i < c.length; i++) if (c[i][1]) met++;
  var bonus = /[^\p{L}\p{Nd}]/u.test(v) ? 1 : 0;
  var long  = Array.from(v).length >= 24 ? 1 : 0;
  var score = met + bonus + long;

  var bar = document.getElementById(meterId);
  bar.style.width = (v === '' ? 0 : (score / 6) * 100) + '%';
  bar.style.background = met < 4 ? '#dc2626' : (score >= 6 ? '#15803d' : (score === 5 ? '#22c55e' : '#f59e0b'));

  var ul = document.getElementById(checksId);
  ul.innerHTML = '';
  if (v === '') return;
  for (var j = 0; j < c.length; j++) {
    var li = document.createElement('li');
    li.textContent = (c[j][1] ? '\u2713 ' : '\u00b7 ') + c[j][0];
    li.className = c[j][1] ? 'met' : '';
    ul.appendChild(li);
  }
  differentHint();
}

// Les deux phrases doivent différer : l'avertissement n'apparaît que si elles
// se ressemblent, pour ne pas faire de bruit quand tout va bien.
function differentHint() {
  var a = document.getElementById('dbRecovery');
  var b = document.getElementById('backupPass');
  var w = document.getElementById('differentWarn');
  if (!a || !b || !w) return;
  w.style.display = (a.value !== '' && a.value === b.value) ? 'block' : 'none';
}

function pickEnc(on) {
  document.getElementById('encOn').checked  = on;
  document.getElementById('encOff').checked = !on;
  document.getElementById('choiceOn').className  = on ? 'choice selected' : 'choice';
  document.getElementById('choiceOff').className = on ? 'choice' : 'choice selected';
  document.getElementById('encFields').style.display = on ? 'block' : 'none';
}

function validEnc() {
  if (!document.getElementById('encOn').checked) return true;
  var v = document.getElementById('dbRecovery').value;
  if (!pstrong(v)) {
    document.getElementById('dbRecovery').focus();
    var e = document.getElementById('errBox');
    e.textContent = 'La phrase de r\u00e9cup\u00e9ration est trop faible : sans elle, une base chiffr\u00e9e ne se rouvre plus.';
    e.style.display = 'block';
    return false;
  }
  document.getElementById('errBox').style.display = 'none';
  return true;
}

function toggleAdvanced() {
  const s = document.getElementById('advancedSection');
  s.style.display = s.style.display === 'block' ? 'none' : 'block';
}

// ── CHE auto-fill ──────────────────────────────────────────────────────────
(function() {
  const cheInput  = document.getElementById('cheNumber');
  const cheHint   = document.getElementById('cheHint');
  const reCHE     = /^CHE[-. ]?\d{3}[. ]?\d{3}[. ]?\d{3}$/i;
  let debounce;

  function setHint(msg, ok) {
    cheHint.textContent = msg;
    cheHint.style.color = ok ? '#15803d' : '#b91c1c';
    cheHint.style.display = msg ? 'block' : 'none';
  }

  cheInput.addEventListener('input', function() {
    clearTimeout(debounce);
    const val = cheInput.value.trim();
    setHint('', false);
    if (!reCHE.test(val)) return;

    setHint('Recherche dans le registre IDE…', true);
    debounce = setTimeout(async () => {
      try {
        // The wizard runs on a different port than the API server, so we
        // call the wizard's own /uid-lookup proxy which forwards to the server.
        const resp = await fetch('/uid-lookup?che=' + encodeURIComponent(val));
        const data = await resp.json();
        if (!resp.ok) {
          setHint(data.error || 'Non trouvé dans le registre.', false);
          return;
        }
        if (data.name) {
          const n = document.getElementById('companyName');
          if (!n.value) n.value = data.name;
        }
        if (data.legal_form) {
          const sel = document.getElementById('legalForm');
          for (let i = 0; i < sel.options.length; i++) {
            if (sel.options[i].value === data.legal_form) {
              sel.selectedIndex = i; break;
            }
          }
        }
        if (data.address_street)      document.getElementById('addressStreet').value     = data.address_street;
        if (data.address_postal_code) document.getElementById('addressPostalCode').value = data.address_postal_code;
        if (data.address_city)        document.getElementById('addressCity').value        = data.address_city;
        setHint('✓ Données pré-remplies depuis le registre IDE', true);
      } catch (e) {
        setHint('Registre IDE inaccessible — saisie manuelle.', false);
      }
    }, 600);
  });
})();

pickEnc(false);

document.getElementById('setupForm').addEventListener('submit', async function(e) {
  e.preventDefault();
  const btn     = document.getElementById('submitBtn');
  const spinner = document.getElementById('spinner');
  const btnText = document.getElementById('btnText');
  const errBox  = document.getElementById('errBox');
  const infoBox = document.getElementById('infoBox');

  errBox.style.display  = 'none';
  infoBox.style.display = 'none';
  btn.disabled          = true;
  btnText.style.display = 'none';
  spinner.style.display = 'block';

  const firstName          = document.getElementById('firstName').value.trim();
  const lastName           = document.getElementById('lastName').value.trim();
  const email              = document.getElementById('email').value.trim();
  const password           = document.getElementById('password').value;
  const port               = document.getElementById('port').value || '8000';
  const companyName        = document.getElementById('companyName').value.trim();
  const legalForm          = document.getElementById('legalForm').value;
  const cheNumber          = document.getElementById('cheNumber').value.trim();
  const addressStreet      = document.getElementById('addressStreet').value.trim();
  const addressPostalCode  = document.getElementById('addressPostalCode').value.trim();
  const addressCity        = document.getElementById('addressCity').value.trim();
  const vatNumber          = document.getElementById('vatNumber').value.trim();
  const iban               = document.getElementById('iban').value.trim();
  const encryptDatabase    = document.getElementById('encOn').checked;
  const dbRecovery         = document.getElementById('dbRecovery').value;
  const backupPassphrase   = document.getElementById('backupPass').value;

  try {
    const resp = await fetch('/setup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        firstName, lastName, email, password, port,
        companyName, legalForm, cheNumber,
        addressStreet, addressPostalCode, addressCity,
        vatNumber, iban,
        encryptDatabase, dbRecovery, backupPassphrase,
        fiscalYearStartMonth: 1,
      }),
    });
    const data = await resp.json();
    if (!resp.ok) {
      errBox.textContent = data.error || 'Une erreur est survenue.';
      errBox.style.display = 'block';
      btn.disabled = false;
      btnText.style.display = 'block';
      spinner.style.display = 'none';
      return;
    }
    if (data.warning) {
      errBox.textContent = data.warning;
      errBox.style.display = 'block';
      setTimeout(() => { window.location.href = data.redirect; }, 6000);
    } else {
      infoBox.textContent = 'Configuration réussie ! Ouverture de LedgerAlps…';
      infoBox.style.display = 'block';
      setTimeout(() => { window.location.href = data.redirect; }, 1500);
    }
  } catch (err) {
    errBox.textContent = 'Impossible de contacter le service de configuration.';
    errBox.style.display = 'block';
    btn.disabled = false;
    btnText.style.display = 'block';
    spinner.style.display = 'none';
  }
});
</script>
</body>
</html>`

type setupRequest struct {
	// Admin account
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	Port      string `json:"port"`
	// Company / tenant
	CompanyName          string `json:"companyName"`
	LegalForm            string `json:"legalForm"`
	AddressStreet        string `json:"addressStreet"`
	AddressPostalCode    string `json:"addressPostalCode"`
	AddressCity          string `json:"addressCity"`
	CheNumber            string `json:"cheNumber"`
	VatNumber            string `json:"vatNumber"`
	IBAN                 string `json:"iban"`
	FiscalYearStartMonth int    `json:"fiscalYearStartMonth"`
	// Protection au repos, choisie à l'installation. C'est le meilleur moment :
	// la base n'existe pas encore, elle naît chiffrée — aucune conversion, aucun
	// redémarrage, et la comptabilité n'est jamais écrite en clair.
	EncryptDatabase  bool   `json:"encryptDatabase"`
	DBRecovery       string `json:"dbRecovery"`
	BackupPassphrase string `json:"backupPassphrase"`
}

// runSetupWizard starts a local HTTP server, opens the browser at the setup
// page, and blocks until setup is complete (or the wizard server stops).
func runSetupWizard() {
	// Pick an available port for the wizard.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		logFatal("cannot start setup wizard: %v", err)
	}
	wizardURL := fmt.Sprintf("http://127.0.0.1:%d", ln.Addr().(*net.TCPAddr).Port)

	done := make(chan struct{})

	mux := http.NewServeMux()

	// Serve the setup HTML page.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t, _ := template.New("setup").Parse(setupHTML)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = t.Execute(w, nil)
	})

	// Proxy UID/IDE lookup to the ZEFIX registry — avoids CORS from the browser.
	mux.HandleFunc("/uid-lookup", func(w http.ResponseWriter, r *http.Request) {
		proxyUIDLookup(w, r)
	})

	// Handle setup form submission.
	mux.HandleFunc("/setup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req setupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "Données invalides.", http.StatusBadRequest)
			return
		}

		// Validate inputs.
		req.FirstName = strings.TrimSpace(req.FirstName)
		req.LastName = strings.TrimSpace(req.LastName)
		req.Email = strings.TrimSpace(req.Email)
		req.CompanyName = strings.TrimSpace(req.CompanyName)
		if req.CompanyName == "" {
			jsonError(w, "La raison sociale est requise.", http.StatusBadRequest)
			return
		}
		if req.FirstName == "" || req.LastName == "" {
			jsonError(w, "Prénom et nom sont requis.", http.StatusBadRequest)
			return
		}
		if req.Email == "" || !strings.Contains(req.Email, "@") {
			jsonError(w, "Adresse e-mail invalide.", http.StatusBadRequest)
			return
		}
		if len(req.Password) < 8 {
			jsonError(w, "Le mot de passe doit contenir au moins 8 caractères.", http.StatusBadRequest)
			return
		}
		// Le port vient du formulaire, donc du réseau. Il finira dans une URL
		// passée à « cmd /c start » et dans un template.JS — voir portValide.
		// Le valider ICI ferme la porte à l'entrée, plutôt que de compter sur
		// la relecture du fichier pour rattraper ce qu'on y a écrit soi-même.
		if req.Port == "" {
			req.Port = "8000"
		}
		if !portValide(req.Port) {
			jsonError(w, "Port invalide : indiquez un nombre de 1 à 65535.", http.StatusBadRequest)
			return
		}

		// Generate a strong JWT secret.
		secret, err := generateSecret()
		if err != nil {
			jsonError(w, "Impossible de générer le secret JWT.", http.StatusInternalServerError)
			return
		}

		// Build config.
		dataDir := appDataDir()
		cfg := &config{
			JWTSecret:      secret,
			SQLitePath:     filepath.Join(dataDir, "ledgeralps.db"),
			Port:           req.Port,
			Debug:          false,
			AllowedOrigins: "http://localhost:" + req.Port,
		}

		// Save config file before starting server.
		if err := saveConfig(cfg); err != nil {
			jsonError(w, "Impossible d'écrire le fichier de configuration: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Protections au repos — AVANT de démarrer le serveur.
		//
		// C'est tout l'intérêt de les demander à l'installation : la base
		// n'existe pas encore. En posant la clé maintenant, elle naît chiffrée.
		// Aucune conversion, aucun redémarrage, et la comptabilité n'est jamais
		// écrite en clair — pas même le temps d'une migration.
		//
		// La phrase des sauvegardes est posée dans le même mouvement, si bien
		// que l'instantané pris au premier démarrage est déjà chiffré.
		//
		// Un échec ici annule l'installation plutôt que de la poursuivre à
		// moitié : quelqu'un qui a demandé le chiffrement et obtient une base
		// en clair ne le saurait pas.
		if req.EncryptDatabase {
			if _, err := db.NewDatabaseKeys(dataDir).Create(req.DBRecovery); err != nil {
				_ = os.Remove(configFilePath())
				jsonError(w, "Impossible de préparer le chiffrement de la base: "+err.Error(),
					http.StatusBadRequest)
				return
			}
		}
		if req.BackupPassphrase != "" {
			if err := db.NewBackupPolicy(dataDir).Set(req.BackupPassphrase); err != nil {
				_ = os.Remove(configFilePath())
				_ = db.NewDatabaseKeys(dataDir).Forget()
				jsonError(w, "Impossible d'enregistrer la phrase de passe des sauvegardes: "+err.Error(),
					http.StatusBadRequest)
				return
			}
		}

		// Start the server.
		_, err = startServer(cfg)
		if err != nil {
			// Rollback config so next launch re-runs the wizard. Les secrets
			// partent avec : une clé sans base et sans configuration ferait
			// échouer la tentative suivante pour une raison incompréhensible.
			_ = os.Remove(configFilePath())
			_ = db.NewDatabaseKeys(dataDir).Forget()
			jsonError(w, "Impossible de démarrer le serveur: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Wait for server to be ready (up to 30 seconds).
		appURL := fmt.Sprintf("http://localhost:%s", cfg.Port)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := waitForServer(ctx, appURL, http.DefaultClient); err != nil {
			// Rollback config so next launch re-runs the wizard.
			_ = os.Remove(configFilePath())
			jsonError(w, "Le serveur ne répond pas — vérifiez server.log dans "+dataDir, http.StatusServiceUnavailable)
			return
		}

		// Bootstrap first admin user + company settings.
		adminName := req.FirstName + " " + req.LastName
		payload := bootstrapPayload{
			Email:                req.Email,
			Name:                 adminName,
			Password:             req.Password,
			CompanyName:          req.CompanyName,
			LegalForm:            req.LegalForm,
			AddressStreet:        req.AddressStreet,
			AddressPostalCode:    req.AddressPostalCode,
			AddressCity:          req.AddressCity,
			AddressCountry:       "CH",
			CheNumber:            req.CheNumber,
			VatNumber:            req.VatNumber,
			IBAN:                 req.IBAN,
			FiscalYearStartMonth: req.FiscalYearStartMonth,
		}
		// A failed bootstrap used to be logged and then followed by a success
		// screen, so a user whose company details never reached the database was
		// told "Configuration réussie" and only discovered the empty form later,
		// in Settings, with no idea why.
		warning := ""
		if err := bootstrapAdmin(appURL, payload); err != nil {
			logWarn("bootstrap failed: %v", err)
			warning = "Le compte a été créé, mais les informations de votre société n'ont pas pu être enregistrées. Vous pourrez les saisir dans Paramètres."
		}

		// Respond with redirect URL.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		payloadOut, _ := json.Marshal(map[string]string{"redirect": appURL, "warning": warning})
		_, _ = w.Write(payloadOut)

		// Signal that setup is done — shut down the wizard server.
		go func() {
			time.Sleep(3 * time.Second)
			close(done)
		}()
	})

	srv := &http.Server{Handler: mux}

	go func() {
		<-done
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	// Open browser slightly after the server starts.
	go func() {
		time.Sleep(600 * time.Millisecond)
		openBrowser(wizardURL)
	}()

	logInfo("Setup wizard listening on %s", wizardURL)
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		logFatal("wizard server error: %v", err)
	}
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	setupLogger()

	cfg, err := loadConfig()
	if err != nil {
		// No config — first run.
		logInfo("No config found at %s — starting setup wizard.", configFilePath())
		runSetupWizard()
		return
	}

	// Config exists — ensure server is running, then open browser.
	appURL := fmt.Sprintf("http://localhost:%s", cfg.Port)

	// If a reinstall sentinel exists, start the server first then show the
	// "configuration preserved" notification page.
	if _, err := os.Stat(reinstalledMarkerPath()); err == nil {
		if !isServerRunning(appURL, http.DefaultClient) {
			logInfo("Starting server after reinstall…")
			if _, err := startServer(cfg); err != nil {
				logFatal("Cannot start server: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := waitForServer(ctx, appURL, http.DefaultClient); err != nil {
				logFatal("Server did not become ready: %v", err)
			}
		}
		runReinstallNotification(appURL)
		// After notification, the browser is already open — nothing more to do.
		return
	}

	if !isServerRunning(appURL, http.DefaultClient) {
		logInfo("Starting server…")
		if _, err := startServer(cfg); err != nil {
			logFatal("Cannot start server: %v", err)
		}
		// Wait for server (up to 20 s).
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := waitForServer(ctx, appURL, http.DefaultClient); err != nil {
			logFatal("Server did not become ready: %v", err)
		}
	}

	logInfo("Opening browser at %s", appURL)
	openBrowser(appURL)
}

// ── Logging (file-based, since there's no console in windowsgui) ─────────────

var logger *log.Logger

func setupLogger() {
	_ = os.MkdirAll(appDataDir(), 0700)
	logPath := filepath.Join(appDataDir(), "launcher.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		logger = log.New(io.Discard, "", 0)
		return
	}
	logger = log.New(f, "", log.LstdFlags)
}

func logInfo(format string, args ...any) {
	if logger != nil {
		logger.Printf("[INFO]  "+format, args...)
	}
}

func logWarn(format string, args ...any) {
	if logger != nil {
		logger.Printf("[WARN]  "+format, args...)
	}
}

func logFatal(format string, args ...any) {
	if logger != nil {
		logger.Printf("[FATAL] "+format, args...)
	}
	os.Exit(1)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, `{"error":%q}`, msg)
}
