package pod

import (
	"context" 
	"fmt"
	"log"
	"time"

	guuid "github.com/google/uuid"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/matryer/try"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type PodArgs struct {
	PodNamespace           string
	Client                 *kubernetes.Clientset
	FileID                 string
	Input                  string
	Output                 string
	InputMount             string
	OutputMount            string
	ReplyTo                string
	RequestProcessingImage string
}

func (podArgs *PodArgs) GetClient() (error){
	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	podArgs.Client = client

	return nil
}

func (pa PodArgs) CreatePod() error {
	podSpec := pa.GetPodObject()

	var pod *core.Pod = nil

	err := try.Do(func(attempt int) (bool, error){
		var err error

		ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
		defer cancel()
		
		pod, err = pa.Client.CoreV1().Pods(pa.PodNamespace).Create(ctx, podSpec, metav1.CreateOptions{}) 

		if err != nil  && attempt < 5{
			time.Sleep((time.Duration(attempt) * 5) * time.Second) // exponential 5 second wait
		}

		return attempt < 5, err // try 5 times
	})

	if err != nil {
		return err
	}

	if err == nil && pod == nil {
		err = fmt.Errorf("Failed to create pod and no error returned")
		return err
	}

	if pod != nil {
		log.Printf("Successfully created Pod")
	}

	return nil
}

func (pa PodArgs) GetPodObject() *core.Pod {
	return &core.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rebuild-" + guuid.New().String(),
			Namespace: pa.PodNamespace,
		},
		Spec: core.PodSpec{
			ImagePullSecrets: []core.LocalObjectReference{{Name: "regcred"}},
			RestartPolicy:    core.RestartPolicyNever,
			Volumes: []core.Volume{
				{
					Name: "sourcedir",
					VolumeSource: core.VolumeSource{
						PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
							ClaimName: "glasswallsource-pvc",
						},
					},
				},
				{
					Name: "targetdir",
					VolumeSource: core.VolumeSource{
						PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
							ClaimName: "glasswalltarget-pvc",
						},
					},
				},
				{
					Name: "request-processing-config",
					VolumeSource: core.VolumeSource{
						ConfigMap: &core.ConfigMapVolumeSource{
							LocalObjectReference: core.LocalObjectReference{
								Name: "request-processing-config",
							},
						},
					},
				},
			},
			Containers: []core.Container{
				{
					Name:            "rebuild",
					Image:           pa.RequestProcessingImage,
					ImagePullPolicy: core.PullIfNotPresent,
					Env: []core.EnvVar{
						{Name: "FileId", Value: pa.FileID},
						{Name: "InputPath", Value: pa.Input},
						{Name: "OutputPath", Value: pa.Output},
						{Name: "ReplyTo", Value: pa.ReplyTo},
					},
					VolumeMounts: []core.VolumeMount{
						{Name: "sourcedir", MountPath: pa.InputMount},
						{Name: "targetdir", MountPath: pa.OutputMount},
						{Name: "request-processing-config", MountPath: "/app/config"},
					},
					Resources: core.ResourceRequirements{
						Limits: core.ResourceList{
							core.ResourceCPU: resource.MustParse("1"),
							core.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
	}
}