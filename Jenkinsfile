pipeline {
  agent {
    kubernetes {
      label 'kubewatch'
      yaml """
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: dind
    image: docker:18.05-dind
    securityContext:
      privileged: true
    volumeMounts:
      - name: dind-storage
        mountPath: /var/lib/docker
  - name: builder
    image: asia.gcr.io/moj-prod/jenkins-builder-infra-production-golang-1.18
    command:
    - sleep
    - infinity
    env:
    - name: DOCKER_HOST
      value: tcp://localhost:2375
    volumeMounts:
      - name: jenkins-sa
        mountPath: /root/.gcp/
  volumes:
    - name: dind-storage
      emptyDir: {}
    - name: jenkins-sa
      secret:
        secretName: jenkins-sa
"""
    }
  }

  stages {
    stage('docker login') {
      steps {
        container('builder') {
            sh 'cat /root/.gcp/jenkins-sa.json | docker login -u _json_key --password-stdin https://asia.gcr.io'
        }
      }
    }

    stage('build') {
      steps {
        container('builder') {
          sh "docker build -t kubewatch ."
        }
      }
    }

    stage('push') {
      environment {
        DOCKER_REPO = "asia.gcr.io/sharechat-production/sharechat/kubewatch"
      }
      when {
        anyOf {
                  branch 'main'
                  branch 'custom_k8s_events_for_webhook_int'
        }
      }
      steps {
        container('builder') {
          sh "docker tag kubewatch:latest $DOCKER_REPO:$GIT_COMMIT"
          sh "docker tag kubewatch:latest $DOCKER_REPO:v1"
          sh "docker push $DOCKER_REPO:$GIT_COMMIT"
          sh "docker push $DOCKER_REPO:v1"
        }
      }
    }
  }
}
