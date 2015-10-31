Configuration uses the TOML format as described here: <https://github.com/toml-lang/toml>.
The file to be edited can be found in:
1. `/etc/gitlab-runner/config.toml` on *nix systems when gitlab-runner is executed as root.
**This is also path for service configuration**, 
1. `~/.gitlab-runner/config.toml` on *nix systems when gitlab-runner is executed as non-root,
1. `./config.toml` on other systems.

### The global section

This defines global settings of multi-runner.

| Setting | Explanation |
| ------- | ----------- |
| `concurrent` | limits how many jobs globally can be run concurrently. The most upper limit of jobs using all defined runners |

Example:

```bash
concurrent = 4
```

### The [[runners]] section

This defines one runner entry.

| Setting | Explanation |
| ------- | ----------- |
| `name`              | not used, just informatory |
| `url`               | CI URL |
| `token`             | runner token |
| `limit`             | limit how many jobs can be handled concurrently by this token. 0 simply means don't limit |
| `executor`          | select how a project should be built, see next section |
| `shell`             | the name of shell to generate the script (default value is platform dependent) |
| `builds_dir`        | directory where builds will be stored in context of selected executor (Locally, Docker, SSH) |
| `environment`       | append or overwrite environment variables |
| `disable_verbose`   | don't print run commands |
| `output_limit`      | set maximum build log size in kilobytes, by default set to 4096 (4MB) |

Example:

```bash
[[runners]]
  name = "ruby-2.1-docker"
  url = "https://CI/"
  token = "TOKEN"
  limit = 0
  executor = "docker"
  builds_dir = ""
  shell = ""
  environment = ["ENV=value", "LC_ALL=en_US.UTF-8"]
  disable_verbose = false
```

### The EXECUTORS

There are a couple of available executors currently.

| Executor | Explanation |
| -------- | ----------- |
| `shell`       | run build locally, default |
| `docker`      | run build using Docker container - this requires the presence of `[runners.docker]` |
| `docker-ssh`  | run build using Docker container, but connect to it with SSH - this requires the presence of `[runners.docker]` and `[runners.ssh]` |
| `ssh`         | run build remotely with SSH - this requires the presence of `[runners.ssh]` |
| `parallels`   | run build using Parallels VM, but connect to it with SSH - this requires the presence of `[runners.parallels]` and `[runners.ssh]` |

### The SHELLS

There are a couple of available shells that can be run on different platforms.

| Shell | Explanation |
| ----- | ----------- |
| `bash`        | generate Bash (Bourne-shell) script. All commands executed in Bash context (default for all Unix systems) |
| `cmd`         | generate Windows Batch script. All commands are executed in Batch context (default for Windows) |
| `powershell`  | generate Windows PowerShell script. All commands are executed in PowerShell context |

### The [runners.docker] section

This defines the Docker Container parameters.

| Parameter | Explanation |
| --------- | ----------- |
| `host`                      | specify custom Docker endpoint, by default `DOCKER_HOST` environment is used or `unix:///var/run/docker.sock` |
| `hostname`                  | specify custom hostname for Docker container |
| `tls_cert_path`             | when set it will use `ca.pem`, `cert.pem` and `key.pem` from that folder to make secure TLS connection to Docker (useful in boot2docker) |
| `image`                     | use this image to run builds |
| `privileged`                | make container run in Privileged mode (insecure) |
| `disable_cache`             | disable automatic |
| `disable_pull`              | disable docker pull when begin running |
| `wait_for_services_timeout` | specify how long to wait for docker services, set to 0 to disable, default: 30 |
| `cache_dir`                 | specify where Docker caches should be stored (this can be absolute or relative to current working directory) |
| `volumes`                   | specify additional volumes that should be mounted (same syntax as Docker -v option) |
| `extra_hosts`               | specify hosts that should be defined in container environment |
| `links`                     | specify containers which should be linked with building container |
| `services`                  | specify additional services that should be run with build. Please visit [Docker Registry](https://registry.hub.docker.com/) for list of available applications. Each service will be run in separate container and linked to the build. |
| `allowed_images`            | specify wildcard list of images that can be specified in .gitlab-ci.yml |
| `allowed_services`          | specify wildcard list of services that can be specified in .gitlab-ci.yml |

Example:

```bash
[runners.docker]
  host = ""
  hostname = ""
  tls_cert_path = "/Users/ayufan/.boot2docker/certs"
  image = "ruby:2.1"
  privileged = false
  disable_cache = false
  wait_for_services_timeout = 30
  cache_dir = ""
  volumes = ["/data", "/home/project/cache"]
  extra_hosts = ["other-host:127.0.0.1"]
  links = ["mysql_container:mysql"]
  services = ["mysql", "redis:2.8", "postgres:9"]
  allowed_images = ["ruby:*", "python:*", "php:*"]
  allowed_services = ["postgres:9.4", "postgres:latest"]
```

#### Volumes in the [runners.docker] section

You can find the complete guide of Docker volume usage [here](https://docs.docker.com/userguide/dockervolumes/).

Let's use some examples to explain how it work (we assume we have a working runners).

##### Example 1: adding a data volume

A data volume is a specially-designated directory within one or more containers that bypasses the Union File System. Data volumes are designed to persist data, independent of the container's life cycle.

```bash
[runners.docker]
  host = ""
  hostname = ""
  tls_cert_path = "/Users/ayufan/.boot2docker/certs"
  image = "ruby:2.1"
  privileged = false
  disable_cache = true
  volumes = ["/path/to/volume/in/container"]
```

This will create a new volume inside the container at /path/to/volume/in/container.

##### Example 2: mount a host directory as a data volume

In addition to creating a volume using you can also mount a directory from your Docker daemon's host into a container. It's usefull when you want to store builds outside the container.

```bash
[runners.docker]
  host = ""
  hostname = ""
  tls_cert_path = "/Users/ayufan/.boot2docker/certs"
  image = "ruby:2.1"
  privileged = false
  disable_cache = true
  volumes = ["/path/to/bind/from/host:/path/to/bind/in/container:rw"]
```

This will use /path/to/bind/from/host of the CI host inside the container at /path/to/bind/in/container.

### The [runners.parallels] section

This defines the Parallels parameters.

| Parameter | Explanation |
| --------- | ----------- |
| `base_name`         | name of Parallels VM which will be cloned |
| `template_name`     | custom name of Parallels VM linked template (optional) |
| `disable_snapshots` | if disabled the VMs will be destroyed after build |

Example:

```bash
[runners.parallels]
  base_name = "my-parallels-image"
  template_name = ""
  disable_snapshots = false
```

### The [runners.ssh] section

This defines the SSH connection parameters.

| Parameter  | Explanation |
| ---------- | ----------- |
| `host`     | where to connect (overridden when using `docker-ssh`) |
| `port`     | specify port, default: 22 |
| `user`     | specify user |
| `password` | specify password |
| `identity_file` | specify file path to SSH private key (id_rsa, id_dsa or id_edcsa). The file needs to be stored unencrypted |

Example:

```
[runners.ssh]
  host = "my-production-server"
  port = "22"
  user = "root"
  password = "production-server-password"
  identity_file = "
```

### Note

If you'd like to deploy to multiple servers using GitLab CI, you can create a single script that deploys to multiple servers or you can create many scripts. It depends on what you'd like to do.
