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
      command: ["/bin/sh", "-c"]
      args:
        - |
          mkdir -p /etc/docker && \
          cp /tmp/docker-config/daemon.json /etc/docker/daemon.json && \
          dockerd-entrypoint.sh &
          tail -f /dev/null # Keep the container running
      securityContext:
        privileged: true
      volumeMounts:
        - name: dind-storage
          mountPath: /var/lib/docker
        - name: docker-config-volume
          mountPath: /tmp/docker-config
    - name: builder
      image: asia.gcr.io/moj-prod/armory-tester:v3
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
    - name: docker-config-volume
      configMap:
        name: docker-config
"""
    }
  }
  environment{
    entity="sharechat"
    region="mumbai"
    tag="kubewatch"
    user=credentials('armory-user')
    password=credentials('armory-password')
    buildarg-DEPLOYMENT_ID = "feed-service-$GIT_COMMIT"
    buildarg-GITHUB_TOKEN = credentials('github-access')
    imagetag="v55"
  }
  stages {
    stage('docker build') {
      environment{
        entity="sharechat"
        region="mumbai"
        tag="kubewatch"
        user=credentials('armory-user')
        password=credentials('armory-password')
      }
      steps {
        container('builder') {
            sh 'armory build'
        }
      }
    }

    stage('push') {
      environment {
        DOCKER_REPO = "sc-mum-armory.platform.internal/sharechat/kubewatch"
      }
      when {
        anyOf {
                  branch 'main'
                  branch 'custom_k8s_events_for_webhook_int'
                  branch 'docker-test'
        }
      }
      steps {
        container('builder') {
          sh "armory push"
        }
      }
    }
  }
}
