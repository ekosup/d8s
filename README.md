# d8s

[Bahasa Indonesia](README.id.md)

A terminal UI for Docker Engine and Docker Swarm that works the way [k9s](https://k9scli.io) does: tables that stay live, keyboard navigation, `:` commands and `/` filters.

```text
 Context: prod-swarm         <:>      Command mode   <enter>  Open       <r>      Restart
 Engine:  27.5.1             </>      Filter         <d>      Inspect    <i>      Image
 API:     1.47               <?>      Help           <y>      YAML       <u>      Rollback
 Swarm:   manager (leader)   <esc>    Back           <l>      Logs       <ctrl-d> Delete
                             <ctrl-c> Quit           <o>      Rollout
                                                     <s>      Scale
┌──────────────────────────────────────── Services[3] ─────────────────────────────────────────┐
│NAME↑           STACK   MODE         REPLICAS  IMAGE                 PORTS         UPDATE  AGE│
│mon_exporter    mon     global            5/5  prom/node-exporter    -             -       41d│
│shop_api        shop    replicated        2/5  registry/api:1.8.2    -             paused  3m │
│shop_web        shop    replicated        3/3  registry/web:4.1.0    8080->80/tcp  -       2d │
└──────────────────────────────────────────────────────────────────────────────────────────────┘
 <stacks> <services>                                                    Scale shop_api: done
```

d8s talks to the Docker API and nothing else. Nothing is installed on your servers, and the `docker` binary is not needed.

## Install

From the [Releases](https://github.com/ekosup/d8s/releases) page, take the file for your platform:

```bash
# Debian / Ubuntu
sudo dpkg -i d8s_<version>_linux_amd64.deb

# Fedora / RHEL
sudo rpm -i d8s_<version>_linux_amd64.rpm

# Linux or macOS, without a package
tar -xzf d8s_<version>_linux_amd64.tar.gz && sudo mv d8s /usr/local/bin/
```

With Homebrew, on macOS or Linux:

```bash
brew install --cask ekosup/tap/d8s
```

Or from source, with Go 1.24 or later:

```bash
go install github.com/ekosup/d8s/cmd/d8s@latest
```

Or from a checkout of this repository, into `~/.local/bin` (or another `PREFIX`):

```bash
make install
```

Check the result:

```bash
d8s version
d8s info        # version, context, engine, and the files in use
```

d8s uses the active Docker context, like `docker` does. If `docker ps` works in your terminal, so does `d8s`.

## The first five minutes

1. Run `d8s`. The container list appears and keeps itself up to date. (Connected to a Swarm manager, the service list comes first; `:c` opens containers.)
2. Move the highlight with `j` / `k` or the arrow keys.
3. Press `l` to see the logs of the highlighted container. `Esc` goes back.
4. Press `/`, type part of a name, `Enter` to filter. `Esc` clears the filter.
5. Press `:`, type `i` and `Enter` to switch to the image list.
6. Press `?` at any time for every key and command.
7. `:q` or `Ctrl-c` quits.

## Commands

Type `:` followed by one of these. Full names and unique prefixes work too (`:containers`, `:cont`).

| Command | View | Needs Swarm |
| --- | --- | --- |
| `:c` | Containers | |
| `:i` | Images | |
| `:v` | Volumes | |
| `:n` | Networks | |
| `:cp` | Compose projects | |
| `:df` | Disk usage | |
| `:ctx` | Docker contexts | |
| `:ev` | Daemon events | |
| `:svc` | Services | yes |
| `:ts` | Tasks | yes |
| `:no` | Nodes | yes |
| `:stk` | Stacks | yes |
| `:sec` | Secrets | yes |
| `:cfg` | Configs | yes |

## Keys

The header always shows the keys of the view you are in, and `?` shows all of them.

**Everywhere**

| Key | Does |
| --- | --- |
| `:` | Command |
| `/` | Filter a table, or search in logs and inspect output |
| `?` | Help |
| `Enter` | Open what the row contains (from a service to its tasks, for example) |
| `Esc` | Back; clears a filter and marks first |
| `d`, `y` | Inspect as JSON, or as YAML |
| `Shift` + letter | Sort by a column; again to reverse |
| `Space` | Mark rows for a bulk action |
| `Ctrl-d` | Delete, after confirmation |
| `1`–`9` | Switch to that Docker context, on any table |

**Containers**

| Key | Does |
| --- | --- |
| `l` | Logs |
| `s` | Shell (tries `bash`, then `sh`) |
| `m` | Live CPU, memory, network and disk statistics |
| `a`, `x`, `r`, `p` | Start, stop, restart, pause or resume |
| `Ctrl-k` | Kill |
| `h` | Hide or show inactive containers |

**Logs and inspect**

| Key | Does |
| --- | --- |
| `n`, `Shift-n` | Next and previous search hit |
| `w` | Wrap long lines |
| `s` | Pause or resume autoscroll |
| `t` | Show timestamps |
| `0`–`5` | Range: last 1,000 lines, 1, 5, 15, 30 minutes, 1 hour |
| `c` | Copy to the clipboard |
| `Ctrl-s` | Save to `~/.local/state/d8s/dumps/` |

**Swarm**

| View | Key | Does |
| --- | --- | --- |
| Service | `s` | Scale |
| Service | `i` | Change the image |
| Service | `r` | Restart every task |
| Service | `u` | Roll back to the previous spec |
| Service | `o` | Watch a rollout: new and old tasks, status, pause message |
| Service, task | `l` | Logs through the manager, each line prefixed `task@node` |
| Task | `h` | Hide or show task history |
| Task | `s` | Shell, when the container is on the connected node |
| Node | `a` | Availability: active, pause, drain |
| Node | `p` | Promote or demote |
| Node | `b` | Set (`key=value`) or remove (`key-`) a label |

Deleting a service or a stack asks you to type its name.

## Swarm

Swarm views need a connection to a **manager** node. From one manager the whole cluster is visible and can be operated, including the logs of tasks on other nodes.

Two things only reach containers on the node you are connected to, because that is how the Docker API works: shells and statistics. For a task on another node d8s explains why and, when a Docker context is named like that node's hostname, offers to switch to it. For the same reason the container list (`:c`) shows the containers of the connected node only; the cluster as a whole is in `:svc`, `:ts` and `:stk`.

Connecting to another server uses an ordinary Docker context:

```bash
docker context create prod --docker host=ssh://user@manager.example.com
d8s --context prod
```

Inside d8s there are three ways to switch context without leaving:

- `1` to `9` on any table. The numbers follow the order of the context list and are shown in the header when the terminal is wide enough, and always under `?`.
- `:ctx prod`, or any unique start of the name (`:ctx pr`); `Tab` completes it.
- `:ctx`, then `Enter` on a row. That list also shows which contexts are read-only and which are marked as production.

## Read-only mode

```bash
d8s --readonly
```

Every action that changes something is refused, shells included; looking, searching and reading logs still work. The header shows `READ-ONLY`. The mode can also be made permanent per context in the configuration.

## Configuration

Optional. The file is `~/.config/d8s/config.yaml` (or `$XDG_CONFIG_HOME/d8s/config.yaml`, or the path given by `--config` / `$D8S_CONFIG`). Without a file, the defaults below apply.

```yaml
refresh: 2s              # how often a view checks again; at least 500ms
defaultView: auto        # first view; auto = services on a Swarm manager, containers otherwise
logBuffer: 5000          # lines a log page keeps
logTail: 1000            # lines fetched when a log page opens
shell: ""                # shell in containers; empty = bash, then sh
readOnly: false          # refuse every change, on every context
skin: dark               # dark, light, or mono

contexts:
  prod:
    readOnly: true       # this context is always read-only
    production: true     # marked in the header

aliases:
  web: services /shop_web   # :web opens services, filtered

hotkeys:
  f2: svc                   # F2 runs :svc
  ctrl-w: web

views:
  containers:
    columns: [NAME, STATE, CPU%, MEM, AGE]   # columns to show, in this order
```

A mistake in this file is reported with its line number, and d8s does not start until it is fixed. An alias or hotkey that clashes with a built-in one is ignored and reported at start-up. So is a name under `contexts` that is not a Docker context: its settings would apply to nothing, and `d8s info` lists it as well.

An alias or hotkey can name a context: `prod: ctx prod` makes `:prod` switch to it.

The sort order of each view is remembered between sessions on its own, in `~/.local/state/d8s/state.yaml`.

**No colour.** The `mono` skin, or the `NO_COLOR` environment variable, turns all colour off; statuses stay distinguishable through bold, underline and dim.

## For k9s users

The same: `:` commands, `/` filter, `?` help, `Enter` open, `Esc` back, `d` describe, `y` YAML, `l` logs, `s` shell, `Ctrl-d` delete, `Ctrl-k` kill, `Space` mark, `Shift` + letter to sort, `:ctx` to change context.

Different:

| k9s | d8s | Note |
| --- | --- | --- |
| `:po`, `:deploy` | `:c`, `:svc` | A pod is roughly a container or a task; a deployment is roughly a service |
| `:ns` | — | Docker has no namespaces; a stack (`:stk`) is the closest thing |
| `0`–`9` switch namespace | `1`–`9` switch context | Same place in the header |
| `e` edit | — | d8s does not edit specs; there is `s` to scale and `i` to change the image |
| `s` on a deployment = scale | `s` on a service = scale | On a container, `s` is still shell |
| `Ctrl-a` alias list | `?` | The command list is on the help screen |

## Limits

- It creates nothing: deploying a stack and creating services, secrets or configs is done with `docker`.
- It does not build or push images.
- CPU and memory statistics are for containers on the connected daemon only, and are switched off above 100 running containers.
- The interactive shell needs `/dev/tty`, so it does not work on Windows yet.
- Tested against Docker Engine 20.10 (API 1.41) through 29; Podman is not supported.

## Development

```bash
make tools             # install the linter and the release tool into bin/tools
make build             # bin/d8s
make install           # copy it to ~/.local/bin
make test lint         # unit tests and lint
make demo-up           # d8s-demo* containers, network and volume to try things on
make swarm-up          # a three-node Swarm in containers, context d8s-swarm
make test-integration  # tests against that cluster
make test-matrix       # the same against the oldest supported and the latest engine
make bench             # measure against the performance targets
make release-snapshot  # every release artefact into dist/, publishing nothing
make release-status    # what is unreleased, and which release would come next
make release-next PART=patch   # test, bump, tag, push and publish (token from .env)
```

`make swarm-up` does not make your own daemon a Swarm member: the cluster lives in three containers. The product specification is in [docs/PRD.md](docs/PRD.md) and the work plan in [docs/BACKLOG.md](docs/BACKLOG.md); both are in Indonesian.

## Licence

[0BSD](LICENSE): use, copy, modify and distribute it for any purpose, with no conditions at all, not even attribution.

Maintained by Eko Supriyono <esup0001@gmail.com>.
