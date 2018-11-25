package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"github.com/docopt/docopt-go"
	copy2 "github.com/otiai10/copy"
	"gopkg.in/resty.v1"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const SwaggerReleasesUrl = "https://api.github.com/repos/swagger-api/swagger-ui/releases/latest"
const Usage = `
Swagger UI Downloader

Usage:
  sud [--config=<file>] [--out=<dir>] [--spec=<spec_file>]

Options:
  -h --help  		Show help screen and exits
  --config=<file>  	File to read previously downloaded package information (relative)
  --out=<dir>  		Directory to store output to (relative) [default: ./]
  --spec=<file>  	File containing OpenAPI yml/json specification (relative)
`

type GithubResponse struct {
	Url        string `json:"url"`
	TagName    string `json:"tag_name"`
	TarballUrl string `json:"tarball_url"`
}

type Args struct {
	Out string
	Spec   string
	Config string
}

type ConfigFile struct {
	Version string
}

func main() {
	var args Args
	parseArgs(&args)

	// 1. request to get latest swagger-ui release
	logn("Fetching latest release information...")
	res, err := resty.R().Get(SwaggerReleasesUrl)
	if err != nil {
		log.Fatalf("Error has occurred in fetching Swagger-ui latest release details:\n%s\n.Exitting...\n", err.Error())
	}

	var githubRes GithubResponse
	if err := json.Unmarshal([]byte(res.String()), &githubRes); err != nil {
		log.Fatalf("Error has occurred in parsing response.\nExitting...\n")
	}

	currPath, _ := os.Executable()
	currDir := filepath.Dir(currPath)
	data, err := ioutil.ReadFile(path.Join(currDir, args.Config))
	var configVersionParts [3]string
	if err == nil && len(string(data)) > 0 {
		var yml ConfigFile
		err = yaml.Unmarshal(data, &yml)
		if err == nil {
			if isValidSemver := parseSemver(yml.Version, configVersionParts); !isValidSemver {
				fmt.Printf("Failed to parse config file version. Skipping...")
			}
		} else {
			fmt.Printf("Failed to parse config file '%s'. Skipping...", args.Config)
		}

	}

 	configVersionParts = [3]string{"0", "0", "0"}
	releasedSemverParts := sanitizeSemver(githubRes.TagName)

	if diffSemvers(configVersionParts, releasedSemverParts) != 1 {
		fmt.Printf("No update available.\nExtting...")
		os.Exit(0)
	}

	fmt.Println("New version exists.")
	fmt.Printf("Downloading version %s...\n", githubRes.TagName)
	downloadedTarFilename := "swagger-ui_"+githubRes.TagName+".tar.gz"
	err = downloadTheTarball(downloadedTarFilename, githubRes.TarballUrl)
	if err != nil {
		log.Fatalf("Error in downloading latest release: %s\nExitting...", err.Error())
		os.Exit(1)
	}
	fmt.Println("Download successful.")
	fmt.Println("Extracting downloaded zip file...")
	gzReader, _ := os.Open(downloadedTarFilename)
	err = Untar("./extracted", gzReader)
	if err != nil {
		log.Fatalf("Error in extracting tar file: %s\nExitting...", err.Error())
		os.Exit(1)
	}

	fmt.Println("Extracted successfully.")
	fmt.Printf("Copying files to %s\n", args.Out)

	subdirs, _ := ioutil.ReadDir("./extracted/")

	err = copy2.Copy("./extracted/"+subdirs[0].Name()+"/dist", path.Join(currDir, args.Out))
	if err != nil {
		log.Fatalf("Error in copying files: %s\nExitting...", err.Error())
	}

	os.RemoveAll("./extracted")
	os.Remove(downloadedTarFilename)
}

func downloadTheTarball(filename, url string) error {
	r := resty.New()
	r.SetMode("http")
	response, err := r.NewRequest().Get(url)
	if err != nil {
		return err
	}

	er := ioutil.WriteFile(filename, response.Body(), 0644)
	if er != nil {
		return err
	}
	return nil
}

func parseArgs(args *Args) {
	arguments, err := docopt.ParseDoc(Usage)

	if err != nil {
		log.Fatalf("Error occurred in parsing arguments: %v\n", err)
	}

	args.Config, _ = arguments.String("--config")
	args.Out, _ = arguments.String("--out")
	if len(args.Out) == 0 {
		args.Out = "./"
	}
	args.Spec, _ = arguments.String("--spec")
}

func diffSemvers(x, y [3]string) int {
	for i := range x {
		if x[i] > y[i] {
			return 1
		} else if x[i] < y[i] {
			return -1
		}
	}
	return 0
}

func sanitizeSemver(v string) [3]string {
	if strings.Index(v, "v") == 0 {
		v = v[1:]
	}
	var parts [3]string
	ok := parseSemver(v, parts)
	if !ok {
		log.Fatalf("Error in parsing Swagger UI latest release version '%s'", v)
		os.Exit(1)
	}
	return parts
}

func parseSemver(version string, parts [3]string) bool {
	p := strings.Split(version, ".")
	for _, part := range p {
		v, err := strconv.Atoi(part)
		if err != nil || v < 0 {
			return false
		}
	}
	return true
}

func logn(message string) {
	fmt.Println(message)
}

func logf(message ...string) {
	fmt.Printf(message[0], message[1:])
}


func Untar(dst string, r io.Reader) error {

	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dst, header.Name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()
		}
	}
}
