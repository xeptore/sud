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
const SavingFileName = ".sud"
const Usage = `
Swagger UI Downloader

Usage:
  sud [--out=<dir>]

Options:
  -h --help  		Show help screen and exits
  --out=<dir>  		Directory to store output to (relative) [default: .]
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
	_, err := os.Stat(path.Join(*outPath, SavingFileName))
	return !os.IsNotExist(err)
}

func getVersionFileData(filePath *string) ([]byte, error) {
	str := path.Join(*filePath, SavingFileName)
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

func extract() error {
	file, err := os.Open(TempTarFilename)
	if err != nil {
		return err
	}
	err = untar(path.Join(".", TempExtractionDirectory), file)
	if err != nil {
		return err
	}
	return nil
}

func copyContentsToOutput(src string, outPath *string) error {
	subdirs, _ := ioutil.ReadDir(src + "/")

	err := copier.Copy(path.Join(src, subdirs[0].Name(), "/dist"), *outPath)
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
	err = ioutil.WriteFile(path.Join(*outPath, SavingFileName), out, 0644)
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
	warn(fmt.Sprintf("specified output directory: (%s)", outputPath))

	if outputDirectoryExists(&outputPath) {
		warn("output directory exists. no need to create it.")
		if doesVersionFileExists(&outputPath) {
			warn("version file found in output directory.")
			warn("reading version file...")
			versionFileData, err := getVersionFileData(&outputPath)
			if err == nil {
				warn("reading version file successful.")
				warn("validating version file...")
				if ok, vFile := isVersionFileValid(&versionFileData); ok {
					if ok, version := doesVersionFileContainVersionKey(&vFile); ok {
						sanitizeVersion(&version)
						if isVersionValueValid(&version) {
							warn("validating version file successful.")
							warn(fmt.Sprintf("previously downloaded version found %s.", version))
							previousVersionParts = *splitSemver(&version)
						} else {
							warn("version file does not have a valid 'version' value.")
						}
					} else {
						warn("version file does not contain 'version' key.")
					}
				} else {
					warn("invalid version file.")
				}
			} else {
				warn("unable to read version file data.")
			}
		} else {
			warn("version file does not exist.")
		}
	} else {
		warn("output directory does not exist.")
		warn("creating output directory...")
		err = createOutputDirectory(&outputPath)
		if err != nil {
			logErrorFatal("unable to create output directory.")
		}
		warn("output directory created.")
	}

	var response GithubResponse
	warn("fetching Swagger UI latest release information...")
	warn(SwaggerReleasesURL)
	err = fetchLatestReleaseInfo(SwaggerReleasesURL, &response)
	if err != nil {
		logErrorFatal("failed to fetch latest Swagger UI release information.")
	}
	warn("fetching latest release information successful.")

	sanitizeVersion(&response.TagName)
	latestReleaseVersionParts := splitSemver(&response.TagName)

	if !existsNewerVersion(&previousVersionParts, latestReleaseVersionParts) {
		warn("no new version found.")
		warn("Exiting...")
		os.Exit(0)
	}
	warn(fmt.Sprintf("new version %s found.", response.TagName))

	warn("downloading latest release...")
	err = downloadTheTarball(&response.TarballURL)
	if err != nil {
		logErrorFatal("error downloading latest release.")
	}
	warn("downloading latest release successful.")
	warn(fmt.Sprintf("extracting downloaded tar file to %s...", outputPath))
	err = extract()
	if err != nil {
		logErrorFatal("error extracting tar file.")
	}
	warn("extracting downloaded tar file successful.")
	warn(fmt.Sprintf("copying necessary files to %s...", outputPath))
	err = copyContentsToOutput(path.Join(".", TempExtractionDirectory), &outputPath)
	if err != nil {
		logErrorFatal("error copying files.")
	}
	warn("copying necessary files successful.")
	warn("cleaning things up...")
	err = clearRemaining()
	if err != nil {
		logErrorFatal("error removing unnecessary files.")
	}
	warn("cleaning things up successful.")
	warn(fmt.Sprintf("saving latest version to %s...", path.Join(outputPath, SavingFileName)))
	err = setLatestVersion(&outputPath, &response.TagName)
	if err != nil {
		logErrorFatal("error saving latest release version.")
	}
	warn("latest version saved successfully.")
	goodLuck("Have a nice day :)")
}

func parseArgs(args *Args) {
	arguments, err := docopt.ParseDoc(Usage)

	if err != nil {
		log.Fatalf("error occurred in parsing arguments: %v\n", err)
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
