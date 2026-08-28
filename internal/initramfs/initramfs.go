// Package initramfs embarque les fichiers d'integration initramfs (hooks et
// scripts init-premount) et sait les installer sur la machine locale.
//
// Ils sont compiles DANS le binaire hop : `hop unlock setup` n'a donc besoin
// d'aucun fichier externe ni acces reseau pour preparer une machine.
package initramfs

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed files
var files embed.FS

// Fichier embarque -> destination sur le systeme.
var targets = []struct {
	src  string
	dst  string
	mode os.FileMode
}{
	{"files/hook-cloudflared", "/etc/initramfs-tools/hooks/cloudflared", 0755},
	{"files/script-cloudflared", "/etc/initramfs-tools/scripts/init-premount/cloudflared", 0755},
	{"files/hook-hop-unlock", "/etc/initramfs-tools/hooks/hop-unlock", 0755},
	{"files/script-hop-unlock", "/etc/initramfs-tools/scripts/init-premount/hop-unlock", 0755},
}

// Conflict decrit un fichier deja present dont le contenu differe de ce que
// hop installerait — donc une integration faite a la main, ou par un autre
// outil, qu'on ecraserait en silence.
type Conflict struct {
	Path string
	Size int64
}

// Conflicts liste les fichiers existants qui seraient ecrases par Install.
// Un fichier identique a ce qu'on installerait n'est PAS un conflit (cas
// d'une simple reexecution du setup).
func Conflicts() ([]Conflict, error) {
	var out []Conflict
	for _, t := range targets {
		info, err := os.Stat(t.dst)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		current, err := os.ReadFile(t.dst)
		if err != nil {
			return nil, err
		}
		want, err := files.ReadFile(t.src)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(current, want) {
			out = append(out, Conflict{Path: t.dst, Size: info.Size()})
		}
	}
	return out, nil
}

// Backup copie les fichiers existants dans dir avant ecrasement.
func Backup(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for _, t := range targets {
		data, err := os.ReadFile(t.dst)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.Base(filepath.Dir(t.dst))+"_"+filepath.Base(t.dst))
		if err := os.WriteFile(dst, data, 0600); err != nil {
			return err
		}
	}
	return nil
}

// Install ecrit les hooks et scripts sur le systeme (necessite root).
func Install() error {
	for _, t := range targets {
		data, err := files.ReadFile(t.src)
		if err != nil {
			return fmt.Errorf("fichier embarque %s: %w", t.src, err)
		}
		if err := os.WriteFile(t.dst, data, t.mode); err != nil {
			return fmt.Errorf("ecriture %s: %w", t.dst, err)
		}
	}
	return nil
}

// Uninstall retire les hooks et scripts installes par Install.
func Uninstall() error {
	for _, t := range targets {
		if err := os.Remove(t.dst); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("suppression %s: %w", t.dst, err)
		}
	}
	return nil
}

// Rebuild regenere l'image initramfs.
func Rebuild() ([]byte, error) {
	return exec.Command("update-initramfs", "-u").CombinedOutput()
}
