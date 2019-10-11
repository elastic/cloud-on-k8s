package runner

import (
	"fmt"
	"log"
	"strings"
)

// createStorageClass based on default storageclass, creates new, non-default class with "volumeBindingMode: WaitForFirstConsumer"
func createStorageClass() error {
	log.Println("Creating storage class...")

	if exists, err := NewCommand("kubectl get sc").OutputContainsAny("e2e-default"); err != nil {
		return err
	} else if exists {
		return nil
	}

	defaultName := ""
	for _, annotation := range []string{
		`storageclass\.kubernetes\.io/is-default-class`,
		`storageclass\.beta\.kubernetes\.io/is-default-class`,
	} {
		template := `kubectl get sc -o=jsonpath="{$.items[?(@.metadata.annotations.%s=='true')].metadata.name}"`
		baseScs, err := NewCommand(fmt.Sprintf(template, annotation)).OutputList()
		if err != nil {
			return err
		}

		if len(baseScs) != 0 {
			defaultName = baseScs[0]
			break
		}
	}

	if defaultName == "" {
		return fmt.Errorf("default storageclass not found")
	}

	sc, err := NewCommand(fmt.Sprintf("kubectl get sc %s -o yaml", defaultName)).Output()
	if err != nil {
		return err
	}

	sc = strings.Replace(sc, fmt.Sprintf("name: %s", defaultName), "name: e2e-default", -1)
	sc = strings.Replace(sc, "volumeBindingMode: Immediate", "volumeBindingMode: WaitForFirstConsumer", -1)
	err = NewCommand(fmt.Sprintf(`cat <<EOF | kubectl apply -f -
%s
EOF`, sc)).Run()
	if err != nil {
		return err
	}

	// Some providers (AKS) don't allow changing the default. To avoid having two defaults, set newly created storage
	// class to be non-default. Depending on k8s version, a different annotation is needed. To avoid parsing version
	// string, both are set.
	patch := `'{ "metadata": { "annotations": { "storageclass.kubernetes.io/is-default-class":"false", "storageclass.beta.kubernetes.io/is-default-class":"false"} } }'`
	cmd := fmt.Sprintf(`kubectl patch storageclass e2e-default -p %s`, patch)
	return NewCommand(cmd).Run()
}
