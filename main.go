package main

import (
	"context"
	"fmt"
	"github.com/turgabr1/dockerPlexTranscoder/pkg/signals"
	"log"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// movies pvc name
var moviesPVC = os.Getenv("MOVIES_PVC")

// photos pvc name
var photosPVC = os.Getenv("PHOTOS_PVC")

// tv pvc name
var tvPVC = os.Getenv("TV_PVC")

// system pvc name
var configPVC = os.Getenv("CONFIG_PVC")

// system pvc name
var transcodePVC = os.Getenv("TRANSCODE_PVC")

// system pvc name
var sharedPVC = os.Getenv("SHARED_PVC")

// pms namespace
var namespace = os.Getenv("KUBE_NAMESPACE")

// image for the plexmediaserver container containing the transcoder. This
// should be set to the same as the 'master' pms server
var pmsImage = os.Getenv("PMS_IMAGE")
var pmsInternalAddress = os.Getenv("PMS_INTERNAL_ADDRESS")

func main() {
	env := os.Environ()
	args := os.Args

	rewriteEnv(env)
	rewriteArgs(args)
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Error getting working directory: %s", err)
	}
	pod := generatePod(cwd, env, args)

	cfg, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		log.Fatalf("Error building kubeconfig: %s", err)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Error building kubernetes clientset: %s", err)
	}

	pod, err = kubeClient.CoreV1().Pods(namespace).Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		log.Fatalf("Error creating pod: %s", err)
	}

	stopCh := signals.SetupSignalHandler()
	waitFn := func() <-chan error {
		stopCh := make(chan error)
		go func() {
			stopCh <- waitForPodCompletion(kubeClient, pod)
		}()
		return stopCh
	}

	select {
	case err := <-waitFn():
		if err != nil {
			log.Printf("Error waiting for pod to complete: %s", err)
		}
	case <-stopCh:
		log.Printf("Exit requested.")
	}

	log.Printf("Cleaning up pod...")
	err = kubeClient.CoreV1().Pods(namespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
	if err != nil {
		log.Fatalf("Error cleaning up pod: %s", err)
	}
}

// rewriteEnv rewrites environment variables to be passed to the transcoder
func rewriteEnv(in []string) {
	// no changes needed
}

func rewriteArgs(in []string) {
	for i, v := range in {
		switch v {
		case "-progressurl", "-manifest_name", "-segment_list":
			in[i+1] = strings.Replace(in[i+1], "http://127.0.0.1:32400", pmsInternalAddress, 1)
		case "-loglevel", "-loglevel_plex":
			in[i+1] = "debug"
		}
	}
}

func generatePod(cwd string, env []string, args []string) *corev1.Pod {
	envVars := toCoreV1EnvVar(env)
	fmt.Println(moviesPVC)
	fmt.Println(tvPVC)
	fmt.Println(photosPVC)
	fmt.Println(configPVC)
	fmt.Println(sharedPVC)
	fmt.Println(transcodePVC)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "pms-elastic-transcoder-",
		},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{
				"beta.kubernetes.io/arch": "arm64",
			},
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:       "plex",
					Command:    args,
					Image:      pmsImage,
					Env:        envVars,
					WorkingDir: cwd,
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "movies",
							MountPath: "/data/Movies",
							ReadOnly:  true,
						},
						{
							Name:      "photos",
							MountPath: "/data/Photos",
							ReadOnly:  true,
						},
						{
							Name:      "tv",
							MountPath: "/data/Tv",
							ReadOnly:  true,
						},
						{
							Name:      "config",
							MountPath: "/config",
							ReadOnly:  true,
						},
						{
							Name:      "shared",
							MountPath: "/shared",
						},
						{
							Name:      "transcode",
							MountPath: "/tmp/Transcode",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "movies",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: moviesPVC,
						},
					},
				},
				{
					Name: "photos",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: photosPVC,
						},
					},
				},
				{
					Name: "tv",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: tvPVC,
						},
					},
				},
				{
					Name: "config",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: configPVC,
						},
					},
				},
				{
					Name: "shared",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: sharedPVC,
						},
					},
				},
				{
					Name: "transcode",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: transcodePVC,
						},
					},
				},
			},
		},
	}
	fmt.Println(pod)
	return pod
}

func toCoreV1EnvVar(in []string) []corev1.EnvVar {
	out := make([]corev1.EnvVar, len(in))
	for i, v := range in {
		splitvar := strings.SplitN(v, "=", 2)
		out[i] = corev1.EnvVar{
			Name:  splitvar[0],
			Value: splitvar[1],
		}
	}
	return out
}

func waitForPodCompletion(cl kubernetes.Interface, pod *corev1.Pod) error {
	for {
		pod, err := cl.CoreV1().Pods(pod.Namespace).Get(context.Background(), pod.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		switch pod.Status.Phase {
		case corev1.PodPending:
			fmt.Println(pod)
		case corev1.PodRunning:
			fmt.Println(pod.Status)
		case corev1.PodFailed:
			return fmt.Errorf("pod %q failed", pod.Name)
		case corev1.PodSucceeded:
			return nil
		}
		time.Sleep(1 * time.Second)
	}
}
