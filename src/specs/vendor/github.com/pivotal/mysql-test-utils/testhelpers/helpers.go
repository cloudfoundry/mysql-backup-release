package testhelpers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	// nolint
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/cf-test-helpers/commandreporter"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega/gexec"
)

const boshPath = "/usr/local/bin/bosh"
const TestFailureMessage = `
*****************************************

TEST FAILURE

*****************************************
`

func ProperYaml(improperYaml string) []byte {
	return []byte(strings.Replace(improperYaml, "\t", "  ", -1))
}

// CheckForRequiredEnvVars asserts that environment variables in envs must be
// set to a non-empty string. If any environment variable in the slice is not
// set, an error will be returned denoting which variable is unset.
func CheckForRequiredEnvVars(envs []string) {
	var missingEnvs []string

	for _, v := range envs {
		if os.Getenv(v) == "" {
			missingEnvs = append(missingEnvs, v)
		}
	}

	Expect(missingEnvs).To(BeEmpty(), "Missing environment variables: %s", strings.Join(missingEnvs, ", "))
}

func MustSucceed(session *gexec.Session) *gexec.Session {
	stdout := string(session.Out.Contents())
	stderr := string(session.Err.Contents())
	ExpectWithOffset(1, session.ExitCode()).To(BeZero(), fmt.Sprintf("stdout:\n%s\nstderr:\n%s\n", stdout, stderr))
	return session
}

func ExecuteBosh(args []string, timeout time.Duration) *gexec.Session {
	command := exec.Command(boshPath, args...)
	reporter := commandreporter.NewCommandReporter(ginkgo.GinkgoWriter)
	reporter.Report(time.Now(), command)
	session, err := gexec.Start(command, ginkgo.GinkgoWriter, ginkgo.GinkgoWriter)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	session.Wait(timeout)

	return session
}

func ExecuteBoshNoOutput(args []string, timeout time.Duration) *gexec.Session {
	command := exec.Command(boshPath, args...)
	fmt.Fprintf(
		ginkgo.GinkgoWriter,
		"\x1b[32m.\x1b[0m",
	)
	session, err := gexec.Start(command, nil, ginkgo.GinkgoWriter)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	session.Wait(timeout)

	return session
}

type Instance struct {
	IP           string
	Index        string
	UUID         string
	VmCid        string
	ProcessState string
}

func GetMySQLInstancesSortedByIndex(boshDeployment string) []Instance {
	args := []string{
		"-d", boshDeployment,
		"instances",
		"--details",
		"--column=IPs",
		"--column=Index",
		"--column=Instance",
		"--column='VM CID'",
		"--column='Process State'",
		"--json",
	}
	session := ExecuteBoshNoOutput(args, time.Minute)
	ExpectWithOffset(1, session).To(gexec.Exit(0))

	var result struct {
		Tables []struct {
			Rows []struct {
				IP           string `json:"ips"`
				Index        string `json:"index"`
				Instance     string `json:"instance"`
				VmCid        string `json:"vm_cid"`
				ProcessState string `json:"process_state"`
			}
		}
	}

	contents := session.Out.Contents()
	ExpectWithOffset(1, json.Unmarshal(contents, &result)).To(Succeed())

	var instances []Instance
	for _, row := range result.Tables[0].Rows {
		if strings.HasPrefix(row.Instance, "mysql/") {
			parts := strings.Split(row.Instance, "/")
			uuid := parts[1]
			instances = append(instances, Instance{IP: row.IP, Index: row.Index, UUID: uuid, VmCid: row.VmCid, ProcessState: row.ProcessState})
		}
	}

	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Index < instances[j].Index
	})

	return instances
}

// ExecuteMysqlQueryAsAdmin is a convenience function for calling
// ExecuteMysqlQuery as the admin user, asserting that the mysql command exits
// cleanly and returns the output as a string
func ExecuteMysqlQueryAsAdmin(deploymentName, instanceIndex, sqlQuery string) string {
	command := fmt.Sprintf(`mysql --defaults-file=/var/vcap/jobs/mysql/config/mylogin.cnf --silent --silent --execute "%s"`,
		sqlQuery)

	session := MustSucceed(executeMysqlQuery(deploymentName, instanceIndex, command))
	return strings.TrimSpace(string(session.Out.Contents()))
}

// ExecuteMysqlQuery executes sqlQuery against the MySQL deployment denoted by
// deploymentName and instance instanceIndex, using credentials in userName and
// password. It returns a pointer to a gexec.Session to be consumed.
func ExecuteMysqlQuery(deploymentName, instanceIndex, userName, password, sqlQuery string) *gexec.Session {
	command := fmt.Sprintf(`MYSQL_PWD="%s" mysql -u %s --silent --silent --execute "%s"`,
		password,
		userName,
		sqlQuery)

	return executeMysqlQuery(deploymentName, instanceIndex, command)
}

func executeMysqlQuery(deploymentName, instanceIndex, command string) *gexec.Session {
	args := []string{
		"--deployment",
		deploymentName,
		"ssh",
		"mysql/" + instanceIndex,
		"--results",
		"--column=Stdout",
		"--command",
		command,
	}

	return ExecuteBosh(args, 2*time.Minute)
}

// GetManifestValue fetches a value from deploymentName's manifest at the path
// xPath
func GetManifestValue(deploymentName, xPath string) string {
	manifestPath := DownloadManifest(deploymentName)

	defer func() {
		err := os.Remove(manifestPath)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
	}()

	output := Interpolate(manifestPath, xPath)
	return output
}

// DownloadManifest downloads the manifest for deployment deploymentName into a
// temporary file. It returns the filename path as a string. It is up to the
// caller to delete this file when they are done with the manifest.
func DownloadManifest(deploymentName string) string {
	args := []string{
		"--deployment",
		deploymentName,
		"manifest",
	}

	session := ExecuteBosh(args, 2*time.Minute)
	output := session.Out.Contents()

	tmpfile, err := ioutil.TempFile("", "manifest")
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	defer func() {
		e := tmpfile.Close()
		ExpectWithOffset(1, e).ToNot(HaveOccurred())
	}()

	_, err = tmpfile.Write(output)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	return tmpfile.Name()
}

// Interpolate interpolates the xPath selector against the manifest file at
// manifestPath and returns the resulting YAML as a string.
func Interpolate(manifestPath string, selector string) string {
	args := []string{
		"interpolate",
		manifestPath,
		"--path",
		selector,
	}
	session := ExecuteBosh(args, 2*time.Minute)

	output := session.Out.Contents()
	return strings.TrimSpace(string(output))
}

// FindFollower find a follower VM in deploymentName
func FindFollower(deploymentName string) *Instance {
	var (
		showSlaveStatus = `SHOW SLAVE STATUS\\G`
		slaveIoState    = `Slave_IO_State`
		followers       []Instance
	)

	// Find a follower by issuing showSlaveStatus to each instance and grabbing the first that
	// returns a result containing slaveIoState
	for _, vm := range GetMySQLInstancesSortedByIndex(deploymentName) {
		slaveStatus := ExecuteMysqlQueryAsAdmin(deploymentName, vm.UUID, showSlaveStatus)
		if strings.Contains(slaveStatus, slaveIoState) {
			followers = append(followers, vm)
		}
	}

	Expect(len(followers)).To(Equal(1))
	return &followers[0]
}

// FindLeader find a leader VM in deploymentName
func FindLeader(deploymentName string) *Instance {
	var (
		readOnlyQuery = "SELECT @@global.read_only"
		leaders       []Instance
	)

	for _, vm := range GetMySQLInstancesSortedByIndex(deploymentName) {
		if ExecuteMysqlQueryAsAdmin(deploymentName, vm.UUID, readOnlyQuery) == "0" {
			leaders = append(leaders, vm)
		}
	}

	Expect(len(leaders)).To(Equal(1))
	return &leaders[0]
}
