package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"text/template"

	homedir "github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"

	"github.com/Luzifer/rconfig/v2"
)

var (
	cfg = struct {
		ExposedPorts   []string `flag:"expose,e" default:"" description:"Ports to expose (format '8000' or '8000/tcp')"`
		LogLevel       string   `flag:"log-level" default:"info" description:"Log level (debug, info, warn, error, fatal)"`
		TemplateFile   string   `flag:"template-file" default:"~/.config/gen-dockerfile.tpl" description:"Template to use for generating the docker file"`
		Timezone       string   `flag:"timezone,t" default:"" description:"Set timezone in Dockerfile (format 'Europe/Berlin')"`
		VersionAndExit bool     `flag:"version" default:"false" description:"Prints current version and exits"`
		Volumes        []string `flag:"volume,v" default:"" description:"Volumes to create mount points for (format '/data')"`
	}{}

	version = "dev"
)

func init() {
	rconfig.AutoEnv(true)
	if err := rconfig.ParseAndValidate(&cfg); err != nil {
		log.Fatalf("Unable to parse commandline options: %s", err)
	}

	if cfg.VersionAndExit {
		fmt.Printf("gen-dockerfile %s\n", version)
		os.Exit(0)
	}

	if l, err := log.ParseLevel(cfg.LogLevel); err != nil {
		log.WithError(err).Fatal("Unable to parse log level")
	} else {
		log.SetLevel(l)
	}
}

func main() {
	pkg, err := getPackage()
	if err != nil {
		log.WithError(err).Fatal("Could not get package name")
	}

	gitName, err := getGitConfig("user.name")
	if err != nil {
		log.WithError(err).Fatal("Could not get git user.name")
	}
	gitEmail, err := getGitConfig("user.email")
	if err != nil {
		log.WithError(err).Fatal("Could not get git user.email")
	}

	params := map[string]interface{}{
		"binary":   getBinaryName(pkg),
		"expose":   deleteEmpty(cfg.ExposedPorts),
		"git_mail": gitEmail,
		"git_name": gitName,
		"package":  pkg,
		"timezone": cfg.Timezone,
		"volumes":  `"` + strings.Join(cfg.Volumes, `", "`) + `"`,
	}

	tplPath, err := homedir.Expand(cfg.TemplateFile)
	if err != nil {
		log.WithError(err).Fatal("Could not find users homedir")
	}

	tpl, err := template.New("gen-dockerfile.tpl").ParseFiles(tplPath)
	if err != nil {
		log.WithError(err).Fatalf("Could not parse template %q", tplPath)
	}

	buf := new(bytes.Buffer)
	if err := tpl.Execute(buf, params); err != nil {
		log.WithError(err).Fatalf("Could not render template %q", tplPath)
	}

	fmt.Println(strings.TrimSpace(regexp.MustCompile(`\n{3,}`).ReplaceAllString(buf.String(), "\n\n")))
}

func getPackage() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	return strings.Replace(cwd, os.Getenv("GOPATH")+"/src/", "", -1), nil
}

func getBinaryName(pkg string) string {
	parts := strings.Split(pkg, "/")
	return parts[len(parts)-1]
}

func getGitConfig(config string) (string, error) {
	buf := new(bytes.Buffer)

	cmd := exec.Command("git", "config", "--get", config)
	cmd.Stdout = buf
	err := cmd.Run()

	return strings.TrimSpace(buf.String()), err
}

func deleteEmpty(s []string) []string {
	var r []string
	for _, str := range s {
		if str != "" {
			r = append(r, str)
		}
	}
	return r
}
