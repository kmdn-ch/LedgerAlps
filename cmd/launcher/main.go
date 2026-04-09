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
	"strings"
	"time"
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

func loadConfig() (*config, error) {
	f, err := os.Open(configFilePath())
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var c config
	return &c, json.NewDecoder(f).Decode(&c)
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
func waitForServer(ctx context.Context, baseURL string) error {
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
			resp, err := http.DefaultClient.Do(req)
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
func isServerRunning(baseURL string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	resp, err := http.DefaultClient.Do(req)
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

  <form id="setupForm">

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
        <input type="text" id="cheNumber" placeholder="CHE-123.456.789">
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

    <button type="submit" class="btn" id="submitBtn">
      <span id="btnText">Démarrer LedgerAlps</span>
      <div class="spinner" id="spinner"></div>
    </button>
  </form>
</div>

<script>
function toggleAdvanced() {
  const s = document.getElementById('advancedSection');
  s.style.display = s.style.display === 'block' ? 'none' : 'block';
}

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

  try {
    const resp = await fetch('/setup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        firstName, lastName, email, password, port,
        companyName, legalForm, cheNumber,
        addressStreet, addressPostalCode, addressCity,
        vatNumber, iban,
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
    infoBox.textContent = 'Configuration réussie ! Ouverture de LedgerAlps…';
    infoBox.style.display = 'block';
    setTimeout(() => { window.location.href = data.redirect; }, 1500);
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
		req.FirstName   = strings.TrimSpace(req.FirstName)
		req.LastName    = strings.TrimSpace(req.LastName)
		req.Email       = strings.TrimSpace(req.Email)
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
		if req.Port == "" {
			req.Port = "8000"
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

		// Start the server.
		_, err = startServer(cfg)
		if err != nil {
			// Rollback config so next launch re-runs the wizard.
			_ = os.Remove(configFilePath())
			jsonError(w, "Impossible de démarrer le serveur: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Wait for server to be ready (up to 30 seconds).
		appURL := fmt.Sprintf("http://localhost:%s", cfg.Port)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := waitForServer(ctx, appURL); err != nil {
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
		if err := bootstrapAdmin(appURL, payload); err != nil {
			logWarn("bootstrap warning: %v", err)
		}

		// Respond with redirect URL.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"redirect":"%s"}`, appURL)

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

	if !isServerRunning(appURL) {
		logInfo("Starting server…")
		if _, err := startServer(cfg); err != nil {
			logFatal("Cannot start server: %v", err)
		}
		// Wait for server (up to 20 s).
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := waitForServer(ctx, appURL); err != nil {
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
