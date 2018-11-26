package main

import (
	"encoding/json"
	"fmt"
	"github.com/docopt/docopt-go"
	copier "github.com/otiai10/copy"
	"gopkg.in/resty.v1"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
)

const SwaggerReleasesURL = "https://api.github.com/repos/swagger-api/swagger-ui/releases/latest"
const Usage = `
Swagger UI Downloader

Usage:
  sud [--out=<dir>]

Options:
  -h --help  		Show help screen and exits
  --out=<dir>  		Directory to store output to (relative) [default: ./]
`
const TempTarFilename = "temp.tar.gz"
const TempExtractionDirectory = "extracted"

type GithubResponse struct {
	URL        string `json:"url"`
	TagName    string `json:"tag_name"`
	TarballURL string `json:"tarball_url"`
}

type Args struct {
	Out    string
	Strict string
}

type VersionFile struct {
	Version string
}

func getAbsoluteOutputPath(relativeOutputPath *string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	return path.Join(dir, *relativeOutputPath), nil
}

func outputDirectoryExists(outPath *string) bool {
	_, err := os.Stat(*outPath)
	return !os.IsNotExist(err)
}

func doesVersionFileExists(outPath *string) bool {
	_, err := os.Stat(path.Join(*outPath, ".sud"))
	return !os.IsNotExist(err)
}

func getVersionFileData(filePath *string) ([]byte, error) {
	str := path.Join(*filePath, ".sub")
	data, err := ioutil.ReadFile(str)
	if err != nil {
		return []byte{}, err
	}
	return data, nil
}

func isVersionFileValid(fileData *[]byte) (bool, VersionFile) {
	var vFile VersionFile
	err := yaml.Unmarshal(*fileData, &vFile)
	if err != nil {
		return false, VersionFile{}
	}
	return true, vFile
}

func doesVersionFileContainVersionKey(vFile *VersionFile) (bool, string) {
	if len(vFile.Version) > 0 {
		return true, vFile.Version
	}
	return false, ""
}

func isVersionValueValid(version *string) bool {
	parts := strings.Split(*version, ".")
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return false
		}
	}
	return true
}

func splitSemver(version *string) *[]string {
	splitted := strings.Split(*version, ".")
	return &splitted
}

func fetchLatestReleaseInfo(url string, response *GithubResponse) error {
	res, err := resty.R().Get(url)
	if err != nil {
		return err
	}
	err = json.Unmarshal([]byte(res.String()), response)
	if err != nil {
		return err
	}
	return nil
}

func existsNewerVersion(previous, current *[]string) bool {
	for i := range *current {
		if (*current)[i] > (*previous)[i] {
			return true
		}
	}
	return false
}

func downloadTheTarball(url *string) error {
	r := resty.New()
	r.SetMode("http")
	res, err := r.NewRequest().Get(*url)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(TempTarFilename, res.Body(), 0644)
	if err != nil {
		return err
	}
	return nil
}

func extract(outPath *string) error {
	file, err := os.Open(TempTarFilename)
	if err != nil {
		return err
	}
	err = untar(path.Join(*outPath, TempExtractionDirectory), file)
	if err != nil {
		return err
	}
	return nil
}

func copyContentsToOutput(outPath *string) error {
	subdirs, _ := ioutil.ReadDir("./"+ TempExtractionDirectory +"/")

	err := copier.Copy("./" + TempExtractionDirectory + "/" + subdirs[0].Name() + "/dist", *outPath)
	if err != nil {
		return err
	}
	return nil
}

func clearRemaining() error {
	err := os.RemoveAll("./" + TempExtractionDirectory + "")
	err = os.Remove(TempTarFilename)
	if err != nil {
		return err
	}
	return nil
}

func setLatestVersion(outPath, version *string) error {
	versionFileData := VersionFile{Version: *version}
	out, err := yaml.Marshal(versionFileData)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(path.Join(*outPath, ".sud"), out, 0644)
	if err != nil {
		return err
	}
	return nil
}

func createOutputDirectory(outPath *string) error {
	err := os.MkdirAll(*outPath, 0755)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	previousVersionParts := []string{"0", "0", "0"}
	var arg Args
	parseArgs(&arg)
	outputPath, err := getAbsoluteOutputPath(&arg.Out)
	
	if outputDirectoryExists(&outputPath) {
		if doesVersionFileExists(&outputPath) {
			versionFileData, err := getVersionFileData(&outputPath)
			if err != nil {
				fmt.Println("Error reading version file data")
			}

			if ok, vFile := isVersionFileValid(&versionFileData); ok {
				if ok, version := doesVersionFileContainVersionKey(&vFile); ok {
					sanitizeVersion(&version)
					if isVersionValueValid(&version) {
						previousVersionParts = *splitSemver(&version)
					}
				}
			}
		}
	} else {
		err = createOutputDirectory(&outputPath)
	}

	var response GithubResponse
	err = fetchLatestReleaseInfo(SwaggerReleasesURL, &response)
	sanitizeVersion(&response.TagName)
	// check for errors
	latestReleaseVersionParts := splitSemver(&response.TagName)

	if !existsNewerVersion(&previousVersionParts, latestReleaseVersionParts) {
		os.Exit(0)
	}

	err = downloadTheTarball(&response.TarballURL)
	err = extract(&outputPath)
	err = copyContentsToOutput(&outputPath)
	err = clearRemaining()
	err = setLatestVersion(&outputPath, &response.TagName)

	if err != nil {
		fmt.Println("Error:", err.Error())
	}
}

func parseArgs(args *Args) {
	arguments, err := docopt.ParseDoc(Usage)

	if err != nil {
		log.Fatalf("Error occurred in parsing arguments: %v\n", err)
	}

	args.Out, _ = arguments.String("--out")
	if len(args.Out) == 0 {
		args.Out = "./"
	}
}

func sanitizeVersion(v *string) {
	tempV := *v
	if strings.Index(*v, "v") == 0 {
		*v = tempV[1:]
	}
}

