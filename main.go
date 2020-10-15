package main

import (
	"context"
	"errors"
	"github.com/guoyk93/conc"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	AnnotationKeyEnabled    = "autoops.auto-replicate-secret/enabled"
	AnnotationKeyOverwrite  = "autoops.auto-replicate-secret/overwrite"
	AnnotationKeyReplicated = "autoops.auto-replicate-secret/replicated"

	PathServiceAccountNamepace = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

var (
	gCurrentNamespace string

	gConfig *rest.Config
	gClient *kubernetes.Clientset

	gLocker          = &sync.Mutex{}
	gKnownNamespaces = map[string]bool{}
	gKnownSecrets    = map[string]*corev1.Secret{}
)

func cloneSecret(s *corev1.Secret, ns string) *corev1.Secret {
	cs := s.DeepCopy()
	rs := &corev1.Secret{
		TypeMeta: cs.TypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:        cs.Name,
			Namespace:   ns,
			Annotations: cs.Annotations,
			Labels:      cs.Labels,
		},
		Data: cs.Data,
		Type: cs.Type,
	}
	// modify annotations
	if rs.Annotations == nil {
		rs.Annotations = map[string]string{
			AnnotationKeyReplicated: "true",
		}
	} else {
		delete(rs.Annotations, AnnotationKeyEnabled)
		delete(rs.Annotations, AnnotationKeyOverwrite)
		rs.Annotations[AnnotationKeyReplicated] = "true"
	}
	return rs
}

func addSecretTo(ctx context.Context, s *corev1.Secret, ns string) {
	log.Printf("---------- ADD %s TO %s", s.Name, ns)
	rs := cloneSecret(s, ns)
	if es, err := gClient.CoreV1().Secrets(ns).Get(ctx, rs.Name, metav1.GetOptions{}); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Printf("failed to check if secret %s existed in %s: %s", rs.Name, ns, err.Error())
			return
		}
		if _, err = gClient.CoreV1().Secrets(ns).Create(ctx, rs, metav1.CreateOptions{}); err != nil {
			log.Printf("failed to replicate %s in %s: %s", rs.Name, ns, err.Error())
		} else {
			log.Printf("replicated %s in %s", rs.Name, ns)
		}
	} else {
		if !shouldOverwriteSecret(es, s) {
			log.Printf("secret %s existed in %s and is not a replicated secret", rs.Name, ns)
			return
		}
		if _, err = gClient.CoreV1().Secrets(ns).Update(ctx, rs, metav1.UpdateOptions{}); err != nil {
			log.Printf("failed to update %s in %s: %s", rs.Name, ns, err.Error())
		} else {
			log.Printf("updated %s in %s", rs.Name, ns)
		}
	}
}

func removeSecretFrom(ctx context.Context, s *corev1.Secret, ns string) {
	log.Printf("---------- REMOVE %s FROM %s", s.Name, ns)
	if es, err := gClient.CoreV1().Secrets(ns).Get(ctx, s.Name, metav1.GetOptions{}); err != nil {
		if k8serrors.IsNotFound(err) {
			log.Printf("secret %s not existed in %s: %s", s.Name, ns, err.Error())
		} else {
			log.Printf("failed to check if secret %s existed in %s: %s", s.Name, ns, err.Error())
		}
	} else {
		if !shouldOverwriteSecret(es, s) {
			log.Printf("secret %s existed in %s and is not a replicated secret", s.Name, ns)
			return
		}
		if err := gClient.CoreV1().Secrets(ns).Delete(ctx, s.Name, metav1.DeleteOptions{}); err != nil {
			log.Printf("failed to remove %s in %s: %s", s.Name, ns, err.Error())
		} else {
			log.Printf("removed %s in %s", s.Name, ns)
		}
	}
}

func addSecret(ctx context.Context, s *corev1.Secret) {
	gLocker.Lock()
	defer gLocker.Unlock()
	gKnownSecrets[s.Name] = s

	for ns := range gKnownNamespaces {
		addSecretTo(ctx, s, ns)
	}
}

func removeSecret(ctx context.Context, s *corev1.Secret) {
	gLocker.Lock()
	defer gLocker.Unlock()
	_, found := gKnownSecrets[s.Name]
	if !found {
		return
	}
	delete(gKnownSecrets, s.Name)

	for ns := range gKnownNamespaces {
		removeSecretFrom(ctx, s, ns)
	}
}

func addNamespace(ctx context.Context, ns string) {
	gLocker.Lock()
	defer gLocker.Unlock()
	gKnownNamespaces[ns] = true

	for _, s := range gKnownSecrets {
		addSecretTo(ctx, s, ns)
	}
}

func removeNamespace(ns string) {
	gLocker.Lock()
	defer gLocker.Unlock()
	delete(gKnownNamespaces, ns)
}

func shouldReplicateNamespace(s *corev1.Namespace) bool {
	if s.Name == gCurrentNamespace {
		return false
	}
	return true
}

func shouldReplicateSecret(s *corev1.Secret) bool {
	if s == nil {
		return false
	}
	if s.Annotations == nil {
		return false
	}
	ok, _ := strconv.ParseBool(s.Annotations[AnnotationKeyEnabled])
	return ok
}

func shouldOverwriteSecret(s *corev1.Secret, src *corev1.Secret) bool {
	if src.Annotations != nil {
		if ok, _ := strconv.ParseBool(src.Annotations[AnnotationKeyOverwrite]); ok {
			return true
		}
	}
	if s == nil {
		return false
	}
	if s.Annotations == nil {
		return false
	}
	ok, _ := strconv.ParseBool(s.Annotations[AnnotationKeyReplicated])
	return ok
}

func exit(err *error) {
	if *err != nil {
		log.Println("exited with error:", (*err).Error())
		os.Exit(1)
	} else {
		log.Println("exited")
	}
}

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ltime | log.Lmsgprefix)

	var err error
	defer exit(&err)

	var buf []byte
	if buf, err = ioutil.ReadFile(PathServiceAccountNamepace); err != nil {
		return
	}

	gCurrentNamespace = strings.TrimSpace(string(buf))

	if gCurrentNamespace == "" {
		err = errors.New("missing current namespace")
		return
	}

	if gConfig, err = rest.InClusterConfig(); err != nil {
		return
	}
	if gClient, err = kubernetes.NewForConfig(gConfig); err != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error, 1)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		errChan <- conc.Parallel(
			routineWatchNamespace(),
			routineWatchSecret(),
		).Do(ctx)
	}()

	select {
	case err = <-errChan:
		return
	case sig := <-sigChan:
		log.Printf("signal caught: %s", sig.String())
		cancel()
		<-errChan
	}

}

func routineWatchNamespace() conc.Task {
	return conc.TaskFunc(func(ctx context.Context) (err error) {
		for {
			var w watch.Interface
			if w, err = gClient.CoreV1().Namespaces().Watch(ctx, metav1.ListOptions{}); err != nil {
				return
			}
			for e := range w.ResultChan() {
				switch e.Type {
				case watch.Added:
					n := e.Object.(*corev1.Namespace)
					if shouldReplicateNamespace(n) {
						log.Printf("++++++++++ NAMESPACE %s ADDED", n.Name)
						addNamespace(ctx, n.Name)
					} else {
						log.Printf("++++++++++ NAMESPACE %s REMOVED", n.Name)
						removeNamespace(n.Name)
					}

				case watch.Deleted:
					n := e.Object.(*corev1.Namespace)
					log.Printf("++++++++++ NAMESPACE %s REMOVED", n.Name)
					removeNamespace(n.Name)

				case watch.Error:
					log.Printf("error occured while watching secrets: %v", e.Object)
				}
			}

			select {
			case <-time.After(time.Second * 10):
				log.Println("restart watching namespaces in 10 seconds")
			case <-ctx.Done():
				return
			}
		}
	})
}

func routineWatchSecret() conc.Task {
	return conc.TaskFunc(func(ctx context.Context) (err error) {
		for {
			var w watch.Interface
			if w, err = gClient.CoreV1().Secrets(gCurrentNamespace).Watch(ctx, metav1.ListOptions{}); err != nil {
				return
			}
			for e := range w.ResultChan() {
				switch e.Type {
				case watch.Added, watch.Modified:
					s := e.Object.(*corev1.Secret)
					if shouldReplicateSecret(s) {
						log.Printf("++++++++++ SECRET %s ADDED", s.Name)
						addSecret(ctx, s)
					} else {
						log.Printf("++++++++++ SECRET %s REMOVED", s.Name)
						removeSecret(ctx, s)
					}
				case watch.Deleted:
					s := e.Object.(*corev1.Secret)
					if shouldReplicateSecret(s) {
						log.Printf("++++++++++ SECRET %s REMOVED", s.Name)
						removeSecret(ctx, s)
					}
				case watch.Error:
					log.Printf("error occured while watching secrets: %v", e.Object)
				}
			}

			select {
			case <-time.After(time.Second * 10):
				log.Println("restart watching secrets in 10 seconds")
			case <-ctx.Done():
				return
			}
		}
	})
}
