package main

// Mode récupération.
//
// Quand la base est chiffrée et que sa clé ne se descelle pas sur ce compte —
// nouveau PC, Windows réinstallé, profil recréé — LedgerAlps ne peut pas ouvrir
// la base. Il s'arrêtait alors avec un message dans un journal que personne ne
// lit, et le point d'entrée de récupération devenait injoignable : le seul
// moyen de retrouver la clé était derrière un serveur qui refusait de démarrer.
//
// Autrement dit, la mesure censée protéger dix ans de pièces (CO art. 958f)
// créait la panne qu'elle devait empêcher. Constaté en supprimant secrets.json
// sur un serveur réel : code de sortie 1, plus rien qui réponde.
//
// LedgerAlps démarre donc ici, sans base, avec une seule page : demander la
// phrase de récupération. Elle desselle la clé, la rescelle à ce compte, et
// relance le serveur normal.

import (
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// runRecoveryServer serves the recovery page until the key is restored, then
// relaunches. It never returns while it succeeds in serving.
func runRecoveryServer(cfg *config.Config, cause error) {
	addr := net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.Port))
	keys := db.NewDatabaseKeys(config.AppDataDir())

	mux := http.NewServeMux()
	render := func(w http.ResponseWriter, status int, message, kind string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(status)
		_ = recoveryPage.Execute(w, map[string]any{
			"Message":     message,
			"Kind":        kind,
			"HasRecovery": keys.HasRecovery(),
			"Cause":       cause.Error(),
		})
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				render(w, http.StatusBadRequest, "Formulaire illisible.", "error")
				return
			}
			phrase := r.PostFormValue("passphrase")
			if phrase == "" {
				render(w, http.StatusUnprocessableEntity, "Indiquez votre phrase de récupération.", "error")
				return
			}
			if _, err := keys.Recover(phrase); err != nil {
				// Le message d'erreur ne distingue pas « mauvaise phrase » de
				// « fichier abîmé » : la différence aiderait surtout quelqu'un
				// qui essaie des phrases au hasard.
				render(w, http.StatusUnprocessableEntity,
					"Cette phrase de récupération ne correspond pas à cette base. "+
						"Vérifiez-la et réessayez.", "error")
				return
			}
			render(w, http.StatusOK,
				"Clé retrouvée et rescellée à ce compte Windows. LedgerAlps redémarre…", "ok")

			// Laisser la page partir avant de couper le processus.
			go func() {
				time.Sleep(1500 * time.Millisecond)
				if err := relaunch(); err != nil {
					log.Printf("FATAL: relance impossible après récupération: %v", err)
					os.Exit(1)
				}
				os.Exit(0)
			}()
			return
		}
		render(w, http.StatusOK, "", "")
	})

	// Un point d'état, pour que la page de redémarrage du navigateur sache que
	// quelque chose répond ici.
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"recovery"}`))
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	fmt.Printf("\nLedgerAlps: la base de données est chiffrée et sa clé n'est pas lisible sur ce compte.\n")
	fmt.Printf("LedgerAlps: ouvrez http://%s pour saisir votre phrase de récupération.\n\n", addr)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("FATAL: %v (cause initiale: %v)", err, cause)
	}
}

// La page est délibérément autonome : le mode récupération tourne sans base et
// doit fonctionner même si rien d'autre ne va.
var recoveryPage = template.Must(template.New("recovery").Parse(`<!doctype html>
<html lang="fr"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>LedgerAlps — Récupération</title>
<style>
 :root { color-scheme: light dark; }
 body { font: 16px/1.6 system-ui, -apple-system, "Segoe UI", sans-serif;
        max-width: 40rem; margin: 3rem auto; padding: 0 1.25rem; }
 h1 { font-size: 1.4rem; margin-bottom: .25rem; }
 .lead { color: #555; margin-top: 0; }
 label { display:block; font-weight:600; margin: 1.5rem 0 .35rem; }
 input { width: 100%; padding: .6rem .7rem; font-size: 1rem;
         border: 1px solid #999; border-radius: 6px; }
 button { margin-top: 1rem; padding: .6rem 1.1rem; font-size: 1rem;
          border: 0; border-radius: 6px; background: #1f4b99; color: #fff; cursor: pointer; }
 .box { border-radius: 6px; padding: .8rem 1rem; margin: 1.25rem 0; }
 .error { background: #fdeaea; border: 1px solid #d66; }
 .ok    { background: #eaf7ee; border: 1px solid #6a6; }
 .note  { background: #f6f6f6; border: 1px solid #ccc; font-size: .92rem; }
 code { font-size: .9em; }
 @media (prefers-color-scheme: dark) {
   .lead { color: #aaa; }
   .error { background:#3a1f1f; border-color:#a55; }
   .ok    { background:#1e3324; border-color:#5a5; }
   .note  { background:#222; border-color:#444; }
   input  { background:#111; color:#eee; border-color:#555; }
 }
</style></head><body>
<h1>Votre comptabilité est chiffrée</h1>
<p class="lead">Elle ne s'ouvre pas avec la clé de ce compte Windows.</p>

{{if .Message}}<div class="box {{.Kind}}">{{.Message}}</div>{{end}}

{{if .HasRecovery}}
<p>C'est le cas prévu : cette base vient d'un autre ordinateur, d'un autre compte
Windows, ou d'une installation de Windows refaite depuis. La clé est enveloppée
dans votre <strong>phrase de récupération</strong> — celle que vous avez notée en
activant le chiffrement.</p>

<form method="post">
  <label for="p">Phrase de récupération</label>
  <input id="p" name="passphrase" type="password" autocomplete="off" autofocus>
  <button type="submit">Récupérer et redémarrer</button>
</form>

<div class="box note">
  <strong>Vous ne l'avez plus ?</strong> La base elle-même est alors définitivement
  illisible. Vos <em>sauvegardes</em>, en revanche, ne dépendent pas de cette clé :
  elles utilisent la phrase de passe des sauvegardes. Installez LedgerAlps
  normalement, puis restaurez la sauvegarde la plus récente.
</div>
{{else}}
<div class="box error">
  Aucune phrase de récupération n'est enregistrée pour cette base : elle ne peut
  pas être rouverte ici. Restaurez une sauvegarde sur une installation neuve —
  les sauvegardes ne dépendent pas de cette clé.
</div>
{{end}}

<div class="box note">Détail technique : <code>{{.Cause}}</code></div>
</body></html>`))
