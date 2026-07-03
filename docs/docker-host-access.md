# Expondo Docker Daemons externos para o Pod Monitor

## Problema

Quando o backend roda dentro de um pod no minikube, qualquer `hostPath` que monte
`/var/run/docker.sock` entrega o socket do **nó do minikube** (dentro da VM),
não o socket da **máquina host** nem de outros daemons remotos.

Para expor qualquer Docker daemon externo ao pod, usamos um **proxy TCP via socat**.

---

## Solução: socat TCP proxy

O socat escuta em um endereço TCP acessível pelo pod e redireciona as conexões para
o Unix socket do daemon desejado.

### Formato geral

```bash
socat TCP-LISTEN:<PORTA>,bind=<IP_ACESSIVEL_PELO_POD>,reuseaddr,fork UNIX-CONNECT:<CAMINHO_DO_SOCKET>
```

| Parâmetro              | Descrição                                                |
|------------------------|----------------------------------------------------------|
| `<PORTA>`              | Porta TCP que o pod vai usar (ex: 2375, 2376, 2377…)    |
| `<IP_ACESSIVEL_PELO_POD>` | IP da interface visível pelo pod (ex: bridge do minikube) |
| `<CAMINHO_DO_SOCKET>`  | Caminho do Unix socket do daemon alvo                    |

---

## Como descobrir o IP da bridge do minikube

```bash
minikube ssh "ip route | grep default | awk '{print \$3}'"
# ou
ip addr show $(ip route | grep $(minikube ip) | awk '{print $3}') | grep 'inet ' | awk '{print $2}' | cut -d/ -f1
```

No setup atual, o IP é `192.168.49.1`.

---

## Configurando como serviço systemd (recomendado)

Crie um arquivo de serviço para cada daemon que quiser expor.

### Exemplo: Docker da máquina host

```ini
# /etc/systemd/system/docker-tcp-proxy.service
[Unit]
Description=Socat TCP proxy para Docker da máquina host
After=network.target

[Service]
ExecStart=/usr/bin/socat TCP-LISTEN:2375,bind=192.168.49.1,reuseaddr,fork UNIX-CONNECT:/var/run/docker.sock
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### Exemplo: segundo daemon Docker (porta diferente)

```ini
# /etc/systemd/system/docker2-tcp-proxy.service
[Unit]
Description=Socat TCP proxy para segundo Docker daemon
After=network.target

[Service]
ExecStart=/usr/bin/socat TCP-LISTEN:2376,bind=192.168.49.1,reuseaddr,fork UNIX-CONNECT:/run/docker2/docker.sock
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### Ativar e iniciar o serviço

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now docker-tcp-proxy.service

# Verificar status
systemctl status docker-tcp-proxy.service
```

---

## Registrando o novo daemon no Pod Monitor

Edite a variável `DOCKER_HOSTS` em `k8s/backend.yaml`:

```yaml
env:
  - name: DOCKER_HOSTS
    value: "minikube-node=/var/run/docker.sock,podman=/run/podman/podman.sock,host-docker=tcp://192.168.49.1:2375"
```

### Formato

```
DOCKER_HOSTS=<nome>=<endereço>,<nome>=<endereço>,...
```

| Tipo de endereço | Formato                | Quando usar                             |
|------------------|------------------------|-----------------------------------------|
| Unix socket      | `/caminho/do/arquivo`  | Daemon rodando no mesmo nó que o pod    |
| TCP              | `tcp://IP:PORTA`       | Daemon remoto ou via socat proxy        |

### Exemplo com múltiplos daemons

```yaml
value: "minikube-node=/var/run/docker.sock,podman=/run/podman/podman.sock,host-docker=tcp://192.168.49.1:2375,servidor-web=tcp://192.168.49.1:2376,servidor-db=tcp://192.168.49.1:2377"
```

Após editar, reaplicar o deployment:

```bash
kubectl apply -f k8s/backend.yaml
```

---

## Classificação automática (cluster vs. host externo)

O backend detecta automaticamente na inicialização se um daemon é um nó de cluster
Kubernetes verificando se algum container possui o label `io.kubernetes.pod.name`.

| Classificação  | Critério                                  | Onde aparece na UI      |
|----------------|-------------------------------------------|-------------------------|
| `cluster=true` | Tem containers com label k8s              | Aba "Containers"        |
| `cluster=false`| Sem containers com label k8s              | Aba "Docker/Podman"     |

---

## Checklist para adicionar um novo daemon

1. [ ] Instalar `socat` na máquina host: `sudo dnf install socat` ou `sudo apt install socat`
2. [ ] Criar o arquivo `.service` em `/etc/systemd/system/`
3. [ ] Escolher uma porta TCP livre (ex: 2375, 2376, 2377…)
4. [ ] Ativar o serviço: `sudo systemctl enable --now <nome>.service`
5. [ ] Testar a conexão: `curl http://192.168.49.1:<PORTA>/v1.41/info`
6. [ ] Adicionar a entrada em `DOCKER_HOSTS` no `k8s/backend.yaml`
7. [ ] Reaplicar: `kubectl apply -f k8s/backend.yaml`
8. [ ] Verificar na UI se o novo host aparece em `/api/docker/hosts`
