package bosh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"e2e-tests/utilities/cmd"
)

// DeployOptionFunc implementations can be passed to DeployManifest
type DeployOptionFunc func(args *[]string)

type MatchInstanceFunc func(instance Instance) bool

type Instance struct {
	IP       string `json:"ips"`
	Instance string `json:"instance"`
	Index    string `json:"index"`
	VMCid    string `json:"vm_cid"`
}

func DeleteDeployment(deploymentName string) error {
	return cmd.Run(
		"bosh",
		"--deployment="+deploymentName,
		"--non-interactive",
		"delete-deployment",
		"--force",
	)
}

func Instances(deploymentName string, matchInstanceFunc MatchInstanceFunc) ([]Instance, error) {
	var output bytes.Buffer

	if err := cmd.RunWithoutOutput(&output,
		"bosh",
		"--non-interactive",
		"--tty",
		"--deployment="+deploymentName,
		"instances",
		"--details",
		"--json",
	); err != nil {
		return nil, err
	}

	var result struct {
		Tables []struct {
			Rows []Instance
		}
	}

	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to decode bosh instances output: %v", err)
	}

	var instances []Instance

	for _, row := range result.Tables[0].Rows {
		if matchInstanceFunc(row) {
			instances = append(instances, row)
		}
	}

	sort.SliceStable(instances, func(i, j int) bool {
		return instances[i].Index < instances[j].Index
	})

	return instances, nil
}

func InstanceIPs(deploymentName string, matchInstanceFunc MatchInstanceFunc) (addresses []string, err error) {
	instances, err := Instances(deploymentName, matchInstanceFunc)
	if err != nil {
		return nil, err
	}

	for _, row := range instances {
		addresses = append(addresses, row.IP)
	}

	return addresses, nil
}

// MatchByInstanceGroup matches by comparing an instance's group against the provided name
func MatchByInstanceGroup(name string) MatchInstanceFunc {
	return func(i Instance) bool {
		components := strings.SplitN(i.Instance, "/", 2)
		return components[0] == name
	}
}

func RemoteCommand(deploymentName, instanceSpec, cmdString string) (string, error) {
	var output bytes.Buffer
	err := cmd.RunWithoutOutput(&output,
		"bosh",
		"--deployment="+deploymentName,
		"ssh",
		instanceSpec,
		"--column=Stdout",
		"--results",
		"--command="+cmdString,
	)
	return strings.TrimSpace(output.String()), err
}

func Scp(deploymentName, sourcePath, destPath string, args ...string) error {
	defaultArgs := []string{"--deployment=" + deploymentName, "scp"}
	defaultArgs = append(defaultArgs, args...)
	defaultArgs = append(defaultArgs, sourcePath)
	defaultArgs = append(defaultArgs, destPath)
	return cmd.Run("bosh", defaultArgs...)
}

func Stop(deploymentName, instanceSpec string, args ...string) error {
	defaultArgs := []string{"-d", deploymentName, "stop"}
	defaultArgs = append(defaultArgs, args...)
	defaultArgs = append(defaultArgs, instanceSpec)
	return cmd.Run("bosh", defaultArgs...)
}

func Start(deploymentName, instanceSpec string, args ...string) error {
	defaultArgs := []string{"-d", deploymentName, "start"}
	defaultArgs = append(defaultArgs, args...)
	defaultArgs = append(defaultArgs, instanceSpec)
	return cmd.Run("bosh", defaultArgs...)
}
