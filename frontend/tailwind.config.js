/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // Palette LedgerAlps — alpestre, neutre, professionnel
        alpine: {
          50:  '#f0f4f8',
          100: '#d9e2ec',
          200: '#bcccdc',
          300: '#9fb3c8',
          400: '#829ab1',
          500: '#627d98',
          600: '#486581',
          700: '#334e68',
          800: '#243b53',
          900: '#102a43',
          950: '#0a1929',
        },
        accent: {
          50:  '#fff8f0',
          100: '#ffefd9',
          200: '#ffdbb0',
          300: '#ffc278',
          400: '#ff9f3d',
          500: '#f97316',
          600: '#ea6c0a',
          700: '#c2570b',
          800: '#9a4510',
          900: '#7c3a12',
        },
        // ── L'écran de connexion ──────────────────────────────────────────
        //
        // Une palette à part, employée sur ce seul écran. C'est la première
        // chose qu'un client voit, et elle vient telle quelle du gabarit
        // validé — l'approcher avec `alpine` et `accent` aurait donné un
        // à-peu-près que personne n'aurait su corriger ensuite.
        //
        // Pas de rouge ici : le drapeau du logo porte le rouge fédéral
        // #D52B1E, et en poser un second, différent, ferait deux marques.
        brand: {
          orange:      '#FF6B1A',
          orangeHover: '#E85A0D',
          navyBg:      '#090F18',
          navyCard:    '#111C2D',
          navyBorder:  '#1E2D42',
          navyInput:   '#162338',
        },
        success: { 500: '#22c55e', 100: '#dcfce7', 700: '#15803d' },
        danger:  { 500: '#ef4444', 100: '#fee2e2', 700: '#b91c1c' },
        warning: { 500: '#f59e0b', 100: '#fef3c7', 700: '#b45309' },
      },
      // Polices système uniquement : LedgerAlps ne charge aucune ressource
      // depuis un tiers. Les familles nommées ici sont présentes sur les
      // systèmes cibles, donc rien n'est téléchargé au chargement de la page.
      // (Auparavant : DM Sans / Syne / JetBrains Mono via fonts.googleapis.com,
      // bloquées par la CSP et donc jamais appliquées.)
      fontFamily: {
        sans: [
          'system-ui', '-apple-system', 'Segoe UI', 'Roboto',
          'Helvetica Neue', 'Arial', 'sans-serif',
        ],
        mono: [
          'ui-monospace', 'SFMono-Regular', 'Cascadia Mono', 'Consolas',
          'Liberation Mono', 'Menlo', 'monospace',
        ],
        display: [
          'system-ui', '-apple-system', 'Segoe UI', 'Roboto',
          'Helvetica Neue', 'Arial', 'sans-serif',
        ],
      },
      borderRadius: {
        DEFAULT: '0.375rem',
        lg: '0.5rem',
        xl: '0.75rem',
      },
      boxShadow: {
        card: '0 1px 3px 0 rgba(16,42,67,.08), 0 1px 2px -1px rgba(16,42,67,.06)',
        modal: '0 20px 60px -10px rgba(16,42,67,.25)',
      },
    },
  },
  plugins: [],
}
