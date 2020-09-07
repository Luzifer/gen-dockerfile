package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"text/template"

	"github.com/fatih/color"
	homedir "github.com/mitchellh/go-homedir"
	diff "github.com/sergi/go-diff/diffmatchpatch"
	log "github.com/sirupsen/logrus"

	"github.com/Luzifer/go_helpers/v2/str"
	"github.com/Luzifer/rconfig/v2"
)

var (
	cfg = struct {
		Diff           bool     `flag:"diff" default:"false" description:"Show a diff to existing Dockerfile"`
		ExposedPorts   []string `flag:"expose,e" default:"" description:"Ports to expose (format '8000' or '8000/tcp')"`
		Feature        []string `flag:"feature,f" default:"" description:"Enable feature defined in template"`
		LogLevel       string   `flag:"log-level" default:"info" description:"Log level (debug, info, warn, error, fatal)"`
		TemplateFile   string   `flag:"template-file" default:"~/.config/gen-dockerfile.tpl" description:"Template to use for generating the docker file"`
		Timezone       string   `flag:"timezone,t" default:"" description:"Set timezone in Dockerfile (format 'Europe/Berlin')"`
		VersionAndExit bool     `flag:"version" default:"false" description:"Prints current version and exits"`
		Volumes        []string `flag:"volume,v" default:"" description:"Volumes to create mount points for (format '/data')"`
		Write          bool     `flag:"write,w" default:"false" description:"Directly write into Dockerfile"`
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

	tpl, err := template.New("gen-dockerfile.tpl").Funcs(templateFuncs()).ParseFiles(tplPath)
	if err != nil {
		log.WithError(err).Fatalf("Could not parse template %q", tplPath)
	}

	buf := new(bytes.Buffer)
	if err := tpl.Execute(buf, params); err != nil {
		log.WithError(err).Fatalf("Could not render template %q", tplPath)
	}

	var output io.Writer = os.Stdout
	if cfg.Diff {
		displayDiff(buf)
		output = ioutil.Discard
	}

	if cfg.Write {
		f, err := os.Create("Dockerfile")
		if err != nil {
			log.WithError(err).Fatal("Could not open Dockerfile for writing")
		}
		defer f.Close()
		output = f
	}

	fmt.Fprintln(output, strings.TrimSpace(regexp.MustCompile(`\n{3,}`).ReplaceAllString(buf.String(), "\n\n")))
}

func displayDiff(buf *bytes.Buffer) {
	var (
		lenOldDockerfile int
		oldDockerfile    []byte
		oldName          = "/dev/null"
	)

	if _, err := os.Stat("Dockerfile"); err == nil {
		oldDockerfile, err = ioutil.ReadFile("Dockerfile")
		if err != nil {
			log.WithError(err).Fatal("Could not read existing Dockerfile")
		}
		lenOldDockerfile = len(bytes.Split(oldDockerfile, []byte{'\n'}))
		oldName = "a/Dockerfile"
	}

	differ := diff.New()
	wSrc, wDst, warray := differ.DiffLinesToRunes(string(oldDockerfile), regexp.MustCompile(`\n{3,}`).ReplaceAllString(buf.String(), "\n\n"))
	diffs := differ.DiffMainRunes(wSrc, wDst, false)
	diffs = differ.DiffCharsToLines(diffs, warray)

	if len(diffs) == 1 && diffs[0].Type == diff.DiffEqual {
		// No diff, everything equal: Nothing to display
		return
	}

	fmt.Printf("--- %s\n", oldName)
	fmt.Println("+++ b/Dockerfile")
	color.Cyan("@@ -0,%d +0,%d @@",
		lenOldDockerfile,
		len(strings.Split(differ.DiffText2(diffs), "\n")),
	)

	for _, d := range diffs {
		text := d.Text
		if text[len(text)-1] == '\n' {
			text = text[:len(text)-1]
		}

		for _, l := range strings.Split(text, "\n") {
			switch d.Type {
			case diff.DiffInsert:
				color.Green("+ %s\n", l)
			case diff.DiffDelete:
				color.Red("- %s\n", l)
			case diff.DiffEqual:
				fmt.Printf("  %s\n", l)
			}
		}
	}
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

func templateFuncs() template.FuncMap {
	return map[string]interface{}{
		"hasFeature": func(name string, v ...string) bool { return str.StringInSlice(name, cfg.Feature) },
	}
}
