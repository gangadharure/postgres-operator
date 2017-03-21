/*
 Copyright 2017 Crunchy Data Solutions, Inc.
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

// Package cmd provides the command line functions of the crunchy CLI
package cmd

import (
	//"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/crunchydata/postgres-operator/tpr"
	"github.com/spf13/viper"
	"io"
	"io/ioutil"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/v1"
	"strings"
	"text/template"
	"time"
)

func showBackup(args []string) {
	//fmt.Printf("showBackup called %v\n", args)

	//show pod information for job
	for _, arg := range args {
		//fmt.Println("show backup called for " + arg)
		//pg-database=basic or
		//pgbackup=true
		if arg == "all" {
			lo := v1.ListOptions{LabelSelector: "pgbackup=true"}
			fmt.Println("label selector is " + lo.LabelSelector)
			pods, err2 := Clientset.Core().Pods(api.NamespaceDefault).List(lo)
			if err2 != nil {
				fmt.Println(err2.Error())
				return
			}
			for _, pod := range pods.Items {
				showItem(pod.Name, "crunchy-pvc")
			}

		} else {
			showItem(arg, "crunchy-pvc")

		}

	}

}
func showItem(name string, pvcName string) {
	//print the pgbackups TPR
	result := tpr.PgBackup{}
	err := Tprclient.Get().
		Resource("pgbackups").
		Namespace(api.NamespaceDefault).
		Name(name).
		Do().
		Into(&result)
	if err == nil {
		fmt.Printf("\npgbackup %s\n", name+" was found ")
	} else if errors.IsNotFound(err) {
		fmt.Printf("\npgbackup %s\n", name+" was not found ")
	} else {
		fmt.Printf("\npgbackup %s\n", name+" lookup error ")
		fmt.Println(err.Error())
	}

	//print the backup jobs if any exists
	lo := v1.ListOptions{LabelSelector: "pg-database=" + name}
	//fmt.Println("label selector is " + lo.LabelSelector)
	pods, err2 := Clientset.Core().Pods(api.NamespaceDefault).List(lo)
	if err2 != nil {
		fmt.Println(err2.Error())
	}
	fmt.Printf("\nbackup job pods for database %s\n", name+"...")
	for _, p := range pods.Items {
		fmt.Printf("%s%s\n", TREE_TRUNK, p.Name)
	}

	//print the database pod if it exists
	var pod *v1.Pod
	pod, err = Clientset.Core().Pods(api.NamespaceDefault).Get(name)
	if err != nil {
		fmt.Printf("\ndatabase pod %s\n", name+" is not found")
		fmt.Println(err.Error())
	} else {
		fmt.Printf("\ndatabase pod %s\n", name+" is found")
	}

	fmt.Println("")

	//print the backups found in the pvc
	printLog(pod.Name, pvcName)
}

func createBackup(args []string) {
	fmt.Printf("createBackup called %v\n", args)

	var err error
	var newInstance *tpr.PgBackup

	for _, arg := range args {
		fmt.Println("create backup called for " + arg)
		result := tpr.PgBackup{}

		// error if it already exists
		err = Tprclient.Get().
			Resource("pgbackups").
			Namespace(api.NamespaceDefault).
			Name(arg).
			Do().
			Into(&result)
		if err == nil {
			fmt.Println("pgbackup " + arg + " was found so we will not create it")
			break
		} else if errors.IsNotFound(err) {
			fmt.Println("pgbackup " + arg + " not found so we will create it")
		} else {
			fmt.Println("error getting pgbackup " + arg)
			fmt.Println(err.Error())
			break
		}
		// Create an instance of our TPR
		newInstance, err = getBackupParams(arg)
		if err != nil {
			fmt.Println("error creating backup")
			break
		}

		err = Tprclient.Post().
			Resource("pgbackups").
			Namespace(api.NamespaceDefault).
			Body(newInstance).
			Do().Into(&result)
		if err != nil {
			fmt.Println("error in creating PgBackup TPR instance")
			fmt.Println(err.Error())
		}
		fmt.Println("created PgBackup " + arg)

	}

}

func deleteBackup(args []string) {
	fmt.Printf("deleteBackup called %v\n", args)
	var err error
	backupList := tpr.PgBackupList{}
	err = Tprclient.Get().Resource("pgbackups").Do().Into(&backupList)
	if err != nil {
		fmt.Println("error getting backup list")
		fmt.Println(err.Error())
		return
	}
	// delete the pgbackup resource instance
	// which will cause the operator to remove the related Job
	for _, arg := range args {
		for _, backup := range backupList.Items {
			if arg == "all" || backup.Spec.Name == arg {
				err = Tprclient.Delete().
					Resource("pgbackups").
					Namespace(api.NamespaceDefault).
					Name(backup.Spec.Name).
					Do().
					Error()
				if err != nil {
					fmt.Println("error deleting pgbackup " + arg)
					fmt.Println(err.Error())
				}
				fmt.Println("deleted pgbackup " + backup.Spec.Name)
			}

		}

	}

}

func getBackupParams(name string) (*tpr.PgBackup, error) {
	var newInstance *tpr.PgBackup

	spec := tpr.PgBackupSpec{}
	spec.Name = name
	spec.PVC_NAME = "crunchy-pvc"
	spec.CCP_IMAGE_TAG = viper.GetString("database.CCP_IMAGE_TAG")
	spec.BACKUP_HOST = "basic"
	spec.BACKUP_USER = "master"
	spec.BACKUP_PASS = "password"
	spec.BACKUP_PORT = "5432"

	//TODO see if name is a database or cluster
	db := tpr.PgDatabase{}
	err := Tprclient.Get().
		Resource("pgdatabases").
		Namespace(api.NamespaceDefault).
		Name(name).
		Do().
		Into(&db)
	if err == nil {
		fmt.Println(name + " is a database")
		spec.PVC_NAME = db.Spec.PVC_NAME
		spec.CCP_IMAGE_TAG = db.Spec.CCP_IMAGE_TAG
		spec.BACKUP_HOST = db.Spec.Name
		spec.BACKUP_USER = db.Spec.PG_MASTER_USER
		spec.BACKUP_PASS = db.Spec.PG_MASTER_PASSWORD
		spec.BACKUP_PORT = db.Spec.Port
	} else if errors.IsNotFound(err) {
		fmt.Println(name + " is not a database")
		cluster := tpr.PgCluster{}
		err = Tprclient.Get().
			Resource("pgclusters").
			Namespace(api.NamespaceDefault).
			Name(name).
			Do().
			Into(&cluster)
		if err == nil {
			fmt.Println(name + " is a cluster")
			spec.PVC_NAME = cluster.Spec.PVC_NAME
			spec.CCP_IMAGE_TAG = cluster.Spec.CCP_IMAGE_TAG
			spec.BACKUP_HOST = cluster.Spec.Name
			spec.BACKUP_USER = cluster.Spec.PG_MASTER_USER
			spec.BACKUP_PASS = cluster.Spec.PG_MASTER_PASSWORD
			spec.BACKUP_PORT = cluster.Spec.Port
		} else if errors.IsNotFound(err) {
			fmt.Println(name + " is not a cluster")
			return newInstance, err
		} else {
			fmt.Println("error getting pgcluster " + name)
			fmt.Println(err.Error())
			return newInstance, err
		}
	} else {
		fmt.Println("error getting pgdatabase " + name)
		fmt.Println(err.Error())
		return newInstance, err
	}

	newInstance = &tpr.PgBackup{
		Metadata: api.ObjectMeta{
			Name: name,
		},
		Spec: spec,
	}
	return newInstance, nil
}

type PodTemplateFields struct {
	Name         string
	CO_IMAGE_TAG string
	BACKUP_ROOT  string
	PVC_NAME     string
}

func printLog(name string, pvcName string) {
	var POD_PATH = viper.GetString("pgo.lspvc_template")
	var PodTemplate *template.Template
	var err error
	var buf []byte
	var doc2 bytes.Buffer
	var podName = "lspvc-" + name

	//delete lspvc pod if it was not deleted for any reason prior
	_, err = Clientset.Core().Pods(api.NamespaceDefault).Get(podName)
	if errors.IsNotFound(err) {
		//
	} else if err != nil {
		fmt.Println(err.Error())
	} else {
		fmt.Println("deleting prior pod " + podName)
		err = Clientset.Core().Pods(api.NamespaceDefault).Delete(podName,
			&v1.DeleteOptions{})
		if err != nil {
			fmt.Println("delete pod error " + err.Error()) //TODO this is debug info
		}
		//sleep a bit for the pod to be deleted
		time.Sleep(2000 * time.Millisecond)
	}

	buf, err = ioutil.ReadFile(POD_PATH)
	if err != nil {
		fmt.Println("error reading lspvc_template file")
		fmt.Println("make sure it is specified in your .pgo.yaml config")
		fmt.Println(err.Error())
		return
	}
	PodTemplate = template.Must(template.New("pod template").Parse(string(buf)))

	podFields := PodTemplateFields{
		Name:         podName,
		CO_IMAGE_TAG: viper.GetString("pgo.CO_IMAGE_TAG"),
		BACKUP_ROOT:  name + "-backups",
		PVC_NAME:     pvcName,
	}

	err = PodTemplate.Execute(&doc2, podFields)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	//podDocString := doc2.String()
	//fmt.Println(podDocString)

	//template name is lspvc-pod.json
	//create lspvc pod
	newpod := v1.Pod{}
	err = json.Unmarshal(doc2.Bytes(), &newpod)
	if err != nil {
		fmt.Println("error unmarshalling json into Pod ")
		fmt.Println(err.Error())
		return
	}
	//var resultPod *v1.Pod
	_, err = Clientset.Core().Pods(v1.NamespaceDefault).Create(&newpod)
	if err != nil {
		fmt.Println("error creating lspvc Pod ")
		fmt.Println(err.Error())
		return
	}
	//fmt.Println("created pod " + resultPod.Name)

	//sleep a bit for the pod to finish, replace later with watch or better
	time.Sleep(3000 * time.Millisecond)

	//get lspvc pod output
	logOptions := v1.PodLogOptions{}
	req := Clientset.Core().Pods(api.NamespaceDefault).GetLogs(podName, &logOptions)
	if req == nil {
		//fmt.Println("error in get logs for " + podName)
	} else {
		//fmt.Println("got the logs for " + podName)
	}

	readCloser, err := req.Stream()
	if err != nil {
		fmt.Println(err.Error())
	}

	defer readCloser.Close()
	var buf2 bytes.Buffer
	_, err = io.Copy(&buf2, readCloser)
	//fmt.Printf("backups are... \n%s", buf2.String())

	fmt.Println("pvc=" + pvcName)
	lines := strings.Split(buf2.String(), "\n")

	//chop off last line since its only a newline
	last := len(lines) - 1
	newlines := make([]string, last)
	copy(newlines, lines[:last])

	for k, v := range newlines {
		if k == len(newlines)-1 {
			fmt.Printf("%s%s\n", TREE_TRUNK, name+"-backups/"+v)
		} else {
			fmt.Printf("%s%s\n", TREE_BRANCH, name+"-backups/"+v)
		}
	}

	//delete lspvc pod
	err = Clientset.Core().Pods(api.NamespaceDefault).Delete(podName,
		&v1.DeleteOptions{})
	if err != nil {
		fmt.Println(err.Error())
		fmt.Println("error deleting lspvc pod " + podName)
	}

}
