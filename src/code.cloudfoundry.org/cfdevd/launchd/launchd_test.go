package launchd_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"code.cloudfoundry.org/cfdevd/launchd"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("launchd", func() {
	Describe("AddDaemon", func() {
		var plistDir string
		var binDir string
		var lnchd launchd.Launchd

		BeforeEach(func() {
			plistDir, _ = ioutil.TempDir("", "plist")
			binDir, _ = ioutil.TempDir("", "bin")
			lnchd = launchd.Launchd{
				PListDir: plistDir,
			}
			ioutil.WriteFile(filepath.Join(binDir, "some-executable"), []byte(`some-content`), 0777)
			session, err := gexec.Start(exec.Command("launchctl", "list"), GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
			Eventually(session).Should(gexec.Exit(0))
			Expect(session.Out.Contents()).ShouldNot(ContainSubstring("org.some-org.some-daemon-name"))
		})

		AfterEach(func() {
			Expect(os.RemoveAll(plistDir)).To(Succeed())
			Expect(os.RemoveAll(binDir)).To(Succeed())
		})

		It("should write the plist, install the binary, and load the daemon", func() {
			installationPath := filepath.Join(binDir, "org.some-org.some-daemon-executable")
			spec := launchd.DaemonSpec{
				Label:            "org.some-org.some-daemon-name",
				Program:          installationPath,
				ProgramArguments: []string{installationPath, "some-arg"},
				RunAtLoad:        true,
			}

			Expect(lnchd.AddDaemon(spec, filepath.Join(binDir, "some-executable"))).To(Succeed())
			plistPath := filepath.Join(plistDir, "/org.some-org.some-daemon-name.plist")
			Expect(plistPath).To(BeAnExistingFile())
			plistFile, err := os.Open(plistPath)
			Expect(err).NotTo(HaveOccurred())
			plistData, err := ioutil.ReadAll(plistFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(plistData)).To(Equal(fmt.Sprintf(
				`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>org.some-org.some-daemon-name</string>
  <key>Program</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>some-arg</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
</dict>
</plist>
`, filepath.Join(binDir, "org.some-org.some-daemon-executable"), filepath.Join(binDir, "org.some-org.some-daemon-executable"))))
			plistFileInfo, err := plistFile.Stat()
			Expect(err).ToNot(HaveOccurred())
			var expectedPlistMode os.FileMode = 0644
			Expect(plistFileInfo.Mode()).To(Equal(expectedPlistMode))

			Expect(installationPath).To(BeAnExistingFile())
			installedBinary, err := os.Open(installationPath)
			Expect(err).NotTo(HaveOccurred())
			binFileInfo, err := installedBinary.Stat()
			var expectedBinMode os.FileMode = 0700
			Expect(binFileInfo.Mode()).To(Equal(expectedBinMode))
			contents, err := ioutil.ReadAll(installedBinary)
			Expect(string(contents)).To(Equal("some-content"))

			session, err := gexec.Start(exec.Command("launchctl", "list"), GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
			defer Expect(exec.Command("launchctl", "unload", plistPath).Run()).To(Succeed())
			Eventually(session).Should(gbytes.Say("org.some-org.some-daemon-name"))
		})
	})

	Describe("RemoveDaemon", func() {
		var (
			plistDir  string
			binDir    string
			plistPath string
			binPath   string
			lnchd     launchd.Launchd
		)

		BeforeEach(func() {
			plistDir, _ = ioutil.TempDir("", "plist")
			binDir, _ = ioutil.TempDir("", "bin")
			plistPath = filepath.Join(plistDir, "org.some-org.some-daemon-to-remove.plist")
			binPath = filepath.Join(binDir, "some-bin-to-remove")
			Expect(ioutil.WriteFile(binPath, []byte("#!/bin bash echo hi"), 0700)).To(Succeed())
			Expect(ioutil.WriteFile(plistPath, []byte(fmt.Sprintf(`
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>org.some-org.some-daemon-to-remove</string>
  <key>Program</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
  </array>
</dict>
</plist>`, binPath)), 0644)).To(Succeed())
			lnchd = launchd.Launchd{
				PListDir: plistDir,
			}
			Expect(exec.Command("launchctl", "load", plistPath).Run()).To(Succeed())
			session, err := gexec.Start(exec.Command("launchctl", "list"), GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
			Eventually(session).Should(gexec.Exit(0))
			Expect(string(session.Out.Contents())).Should(ContainSubstring("org.some-org.some-daemon-to-remove"))
		})

		AfterEach(func() {
			Expect(os.RemoveAll(plistDir)).To(Succeed())
			Expect(os.RemoveAll(binDir)).To(Succeed())
		})

		It("should unload the daemon and remove the files", func() {
			spec := launchd.DaemonSpec{
				Label:            "org.some-org.some-daemon-to-remove",
				Program:          binPath,
				ProgramArguments: []string{binPath},
				RunAtLoad:        true,
			}

			Expect(lnchd.RemoveDaemon(spec)).To(Succeed())
			session, err := gexec.Start(exec.Command("launchctl", "list"), GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
			Eventually(session).Should(gexec.Exit(0))
			Expect(string(session.Out.Contents())).ShouldNot(ContainSubstring("org.some-org.some-daemon-to-remove"))
			Expect(plistPath).NotTo(BeAnExistingFile())
			Expect(binPath).NotTo(BeAnExistingFile())
		})
	})
})
