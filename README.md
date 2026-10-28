<p align="center"><img src="assets/mark.png" width="88" alt="Logo KOL"></p>
<h1 align="center">KOL</h1>
<p align="center">Le fil de vos copies.<br>Un journal de presse-papiers local, discret et chiffré pour Windows.</p>

KOL retient ce que vous copiez et le retrouve au moment où vous en avez besoin. Un clic sur son icône près de l’horloge ouvre l’historique. Recherchez une copie, cliquez dessus, puis collez avec **Ctrl+V** dans votre application.

## Installer

1. Téléchargez **KOL-windows-amd64.zip** depuis les [versions disponibles](https://github.com/nRudzy/KOL/releases/latest).
2. Décompressez le dossier à un emplacement permanent, puis lancez **KOL.exe**.
3. Pour le retrouver, cliquez sur son icône dans la zone de notification Windows, éventuellement sous la flèche des icônes masquées.

Windows 10/11 **64 bits**, avec [Microsoft Edge WebView2 Evergreen Runtime](https://developer.microsoft.com/microsoft-edge/webview2/), généralement déjà installé. KOL est portable : aucun serveur ni compte à configurer. L’exécutable initial est non signé.

## Au quotidien

| Action | Résultat |
| --- | --- |
| Copier dans n’importe quelle application | Ajoute une copie locale, avec ses formats disponibles |
| Recopier un contenu identique | Remonte la même entrée et augmente son compteur |
| Clic gauche sur l’icône / **Ctrl+Alt+V** | Ouvre ou masque le panneau |
| Recherche / **Ctrl+F** | Retrouve texte, lien, nom de fichier, format ou application source |
| Clic sur une copie / **Entrée** | La remet dans le presse-papiers ; **Ctrl+V** la colle ensuite |
| Flèches haut et bas | Sélectionnent une copie dans la liste |
| Épingle | Garde les copies utiles à portée de main |
| Clic droit sur une copie | Affiche le texte brut, les formats et les limites éventuelles |
| **Échap** / clic à l’extérieur | Masque le panneau ; KOL continue à capturer |
| Pause | Suspend la capture ; la consultation reste disponible |
| Clic droit sur l’icône | Ouvre le menu, permet de mettre en pause ou de quitter |

Les paramètres permettent de démarrer avec Windows, de régler la taille maximale d’une copie et d’effacer les entrées non épinglées. Une suppression individuelle fonctionne aussi pour les épingles.

## Fidélité des copies

Le texte est conservé **sans trim ni normalisation** : espaces, tabulations, Unicode et retours à la ligne restent intacts. L’aperçu de la liste est abrégé pour la lisibilité ; le contenu restitué reste complet. La recherche porte sur le texte et les noms de fichiers, sans reconnaissance de caractères dans les images.

KOL conserve plusieurs représentations d’une même copie, dans leur ordre de priorité : texte Unicode/ANSI, liens, HTML, RTF, images DIB/DIBV5/PNG/JPEG, listes de fichiers de l’Explorateur, métafichiers EMF et formats enregistrés transférables en mémoire. Le hachage tient compte des octets et des noms des formats, indépendamment de leur ordre d’énumération. Deux textes identiques avec une mise en forme différente restent distincts.

Windows et certaines applications proposent aussi des objets vivants qui ne sont pas des octets autonomes : flux COM, pièces jointes virtuelles, données privées ou affichées uniquement par l’application source. **Une compatibilité universelle avec tous les objets copiables ne peut pas être garantie.** Les formats non conservés sont indiqués dans le panneau et dans le détail de la copie. Les formats standards disponibles restent utilisables.

Les fichiers sont conservés comme **références vers leur emplacement d’origine**, sans sauvegarder leur contenu. S’ils sont déplacés ou supprimés, l’ancienne copie ne peut pas les recréer. Un ancien « couper » est restitué comme « copier », pour éviter de rejouer un déplacement depuis l’historique. Une taille maximale de **64 Mo par copie**, réglable jusqu’à 512 Mo, protège la réactivité ; les formats dépassant cette limite sont signalés. Aucun effacement automatique par âge ou nombre d’entrées n’est appliqué. Comme tout observateur du presse-papiers, KOL ne peut pas récupérer une copie remplacée par une autre avant que Windows ne lui permette de la lire.

## Données locales

L’historique est enregistré dans **`%LOCALAPPDATA%\KOL`**. Le contenu et les métadonnées sont chiffrés en AES-256-GCM ; une clé protégée par **Windows DPAPI** lie l’ouverture à votre compte Windows. `history.db` et `history.key` sont nécessaires ensemble. Une clé manquante ou une base illisible provoque une erreur explicite, sans remise à zéro silencieuse.

KOL n’utilise aucun service cloud, aucune télémétrie applicative et aucune ressource distante pour son interface. Les signaux Windows d’exclusion du suivi, notamment `CanIncludeInClipboardHistory=0`, sont respectés. Les autres contenus copiés sont enregistrés tant que la capture est active ; le bouton Pause permet de la suspendre.

Pour arrêter KOL, utilisez **Quitter KOL**, et désactivez d’abord le démarrage automatique si vous ne souhaitez plus l’utiliser. Les préférences sont dans `settings.json`. Le dossier `WebView2` contient les données de fonctionnement du moteur d’affichage. Le dossier de données peut être choisi au lancement avec `KOL.exe --data-dir "D:\KOL-data"` ; la clé reste liée au compte Windows.

## Développer

Go **1.26 ou plus récent**. L’application est écrite en Go avec les API Win32, bbolt et WebView2. Le panneau utilise du HTML/CSS/JavaScript embarqué ; aucun environnement Node ni installation de Chromium n’est nécessaire pour compiler ou utiliser KOL.

Sous Windows :

```powershell
go test ./...
go vet ./...
.\scripts\build.ps1
```

Depuis Linux/WSL :

```sh
go test ./...
go vet ./...
./scripts/build.sh
```

L’exécutable est produit dans **`dist/KOL.exe`**, avec son icône et son manifeste Windows. La génération des ressources utilise `rsrc` à une version fixée et fonctionne sans compilateur C. Pour vérifier les accès concurrents au stockage : `go test -race ./...` (nécessite un compilateur C).

Les tests Windows exercent de vrais échanges du presse-papiers dans une station de fenêtres isolée, ainsi que DPAPI et les préférences. Ils ne remplacent pas le presse-papiers de votre session. Les tests portables couvrent conservation des espaces, dédoublonnage, ordre, recherche, chiffrement, persistance, épingles et suppressions. La CI vérifie Linux et Windows, puis construit le binaire Windows.

```text
cmd/kol             Point d’entrée Windows
internal/clipboard  Capture et restitution des formats Win32
internal/history    Identité, aperçus, chiffrement et stockage
internal/desktop    Icône de notification, fenêtre et raccourcis
internal/platform   Clé DPAPI et préférences Windows
internal/ui         Panneau embarqué
assets              Logo, icône et manifeste
```

Licence [MIT](LICENSE). Voir [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) pour les dépendances. Le logo et le concept visuel ont été générés avec un outil de génération d’images puis adaptés aux tailles d’icônes Windows.
