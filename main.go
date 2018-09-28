package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"

	"howett.net/plist"
)

func copyFile(src, dst string) error {
	var err error
	var srcfd *os.File
	var dstfd *os.File
	var srcinfo os.FileInfo

	if srcfd, err = os.Open(src); err != nil {
		return err
	}
	defer srcfd.Close()

	if dstfd, err = os.Create(dst); err != nil {
		return err
	}
	defer dstfd.Close()

	if _, err = io.Copy(dstfd, srcfd); err != nil {
		return err
	}
	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}
	return os.Chmod(dst, srcinfo.Mode())
}

func copyDir(src string, dst string) error {
	var err error
	var fds []os.FileInfo
	var srcinfo os.FileInfo

	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return err
	}

	if fds, err = ioutil.ReadDir(src); err != nil {
		return err
	}
	for _, fd := range fds {
		srcfp := path.Join(src, fd.Name())
		dstfp := path.Join(dst, fd.Name())

		if fd.IsDir() {
			if err = copyDir(srcfp, dstfp); err != nil {
				fmt.Println(err)
			}
		} else {
			if err = copyFile(srcfp, dstfp); err != nil {
				fmt.Println(err)
			}
		}
	}
	return nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}

	if os.IsNotExist(err) {
		return false, nil
	}

	return false, err
}

func combineStaticLibraries(name string, simulatorBuildPath string, osBuildPath string, outputPath string) error {
	frameworkPath := fmt.Sprintf("%v.framework", name)
	libSubpath := fmt.Sprintf("%v.framework/%v", name, name)
	simulatorLibraryPath := path.Join(simulatorBuildPath, libSubpath)
	osLibraryPath := path.Join(osBuildPath, libSubpath)
	combinedLibraryPath := path.Join(osBuildPath, name)

	// Lipo
	cmd := exec.Command("lipo", "-create", simulatorLibraryPath, osLibraryPath, "-output", combinedLibraryPath)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to run lipo to combine static libraries: %v", err)
	}

	// Move combined library back to framework
	err = os.Rename(combinedLibraryPath, osLibraryPath)
	if err != nil {
		return fmt.Errorf("failed to move combined static library: %v", err)
	}

	// Read plist data
	plistPath := path.Join(osBuildPath, fmt.Sprintf("%v.framework/Info.plist", name))
	plistData, err := ioutil.ReadFile(plistPath)
	if err != nil {
		return fmt.Errorf("failed to read info plist file: %v", err)
	}

	var pl map[string]interface{}

	// Unmarshal plist
	format, err := plist.Unmarshal(plistData, &pl)
	if err != nil {
		return fmt.Errorf("failed to unmarshal plist file: %v", err)
	}

	// Changed supported platforms
	pl["CFBundleSupportedPlatforms"] = []string{"iPhoneOS", "iPhoneSimulator"}

	// Marshal plist
	plistData, err = plist.Marshal(pl, format)
	if err != nil {
		return fmt.Errorf("failed to marshal plist file: %v", err)
	}

	// Write back to file
	err = ioutil.WriteFile(plistPath, plistData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write plist file: %v", err)
	}

	// Copy to output
	return copyDir(path.Join(osBuildPath, frameworkPath), path.Join(outputPath, frameworkPath))
}

func main() {
	var user string
	var sdkProjectName string
	var buildConfig string
	var frameworkName string
	var outputFolder string

	flag.StringVar(&user, "user", "", "the OS X user to get to the Xcode derived data path")
	flag.StringVar(&sdkProjectName, "project", "", "the name of the Xcode project")
	flag.StringVar(&buildConfig, "buildconfig", "Release", "the scheme build config for the framework (defaults to Release)")
	flag.StringVar(&frameworkName, "framework", "", "the name of the static library framework")
	flag.StringVar(&outputFolder, "output", "", "the output folder where to copy the result framework")
	flag.Parse()

	if user == "" {
		log.Fatalf("user must be set\n")
	}

	if sdkProjectName == "" {
		log.Fatalf("project must be set\n")
	}

	if frameworkName == "" {
		log.Fatalf("framework must be set\n")
	}

	if outputFolder == "" {
		log.Fatalf("output must be set\n")
	}

	derivedDataPath := path.Join("/Users", user, "/Library/Developer/Xcode/DerivedData")

	exists, err := pathExists(derivedDataPath)
	if err != nil {
		log.Fatalf("failed to os.Stat() derived data folder: %v\n", err)
	}

	if !exists {
		log.Fatalf("can't find Xcode derived data folder\n")
	}

	files, err := ioutil.ReadDir(derivedDataPath)
	if err != nil {
		log.Fatalf("failed to read contents of derived data path: %v\n", err)
	}

	var sdkProjectPath string

	for _, f := range files {
		if strings.HasPrefix(f.Name(), sdkProjectName) {
			sdkProjectPath = path.Join(derivedDataPath, f.Name())
			break
		}
	}

	simulatorBuildPath := path.Join(sdkProjectPath, fmt.Sprintf("Build/Products/%v-iphonesimulator", buildConfig))
	osBuildPath := path.Join(sdkProjectPath, fmt.Sprintf("Build/Products/%v-iphoneos", buildConfig))

	err = combineStaticLibraries(frameworkName, simulatorBuildPath, osBuildPath, outputFolder)
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	log.Printf("created fat library with simulator support")
}
