package main

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
)

func ForwardPortToPod(localPort int32, remotePort int32, podName string) error {
	env := os.Environ()
	procAttr := &os.ProcAttr{
		Env: env,
		Files: []*os.File{
			os.Stdin,
			os.Stdout,
			os.Stderr,
		},
	}

	pid, err := os.StartProcess("/usr/local/bin/kubectl",
		[]string{"kubectl", "port-forward", podName, fmt.Sprintf("%d:%d", localPort, remotePort), "--address=0.0.0.0"}, procAttr)
	if err != nil {
		logrus.Error("Error %v starting process!", err) //
		pid.Kill()
	}
	return nil
}