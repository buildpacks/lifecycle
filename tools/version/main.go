package main

import (
	"fmt"
	"os/exec"
	"regexp"
)

func main() {

	// if the current branch is a release branch, parse the version number from the branch

	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("0.0.0")
		return
	}
	re := regexp.MustCompile("release/(?P<version>.+)")
	matches := re.FindStringSubmatch(string(output))
	if len(matches) == 2 {
		fmt.Println(matches[1])
		return
	}

	// otherwise parse the version from git history (remove leading 'v' and the 'g' in front of commit sha)

	cmd = exec.Command("git", "describe", "--tags")
	output, err = cmd.CombinedOutput()
	if err != nil {
		fmt.Println("0.0.0")
		return
	}
	re = regexp.MustCompile("v(?P<version>.+)-(?P<commits>.+)-g(?P<sha>.+)")
	matches = re.FindStringSubmatch(string(output))
	if len(matches) != 4 {
		fmt.Println("0.0.0")
		return
	}
	fmt.Println(matches[1] + "-" + matches[2] + "+" + matches[3])
}
