

# memsh

[![Test](https://github.com/amjadjibon/memsh/actions/workflows/test.yml/badge.svg)](https://github.com/amjadjibon/memsh/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/amjadjibon/memsh.svg)](https://pkg.go.dev/github.com/amjadjibon/memsh)
[![Go Report Card](https://goreportcard.com/badge/github.com/amjadjibon/memsh)](https://goreportcard.com/report/github.com/amjadjibon/memsh)
[![codecov](https://codecov.io/gh/amjadjibon/memsh/branch/main/graph/badge.svg)](https://codecov.io/gh/amjadjibon/memsh)

Un shell virtual de bash implementado en Go. `memsh` ejecuta comandos similares a bash contra un sistema de archivos en memoria: el sistema de archivos real del sistema operativo nunca se modifica y los comandos externos del SO están bloqueados de forma predeterminada.

El análisis e interpretación del shell es manejado por [mvdan.cc/sh/v3](https://github.com/mvdan/sh). Cada comando es un plugin nativo de Go o un plugin WASM: no hay dependencia de binarios del sistema host.

## Características

- **Ejecución en entorno aislado** — los comandos externos del sistema operativo están bloqueados; solo pueden ejecutarse los plugins registrados
- **Sistema de archivos en memoria** — todas las operaciones de archivos apuntan a `afero.MemMapFs`; nada toca tu disco
- **Sintaxis similar a bash** — tuberías (pipes), redirecciones (`>`, `>>`), `&&`, `;`, subshells, alias
- **Más de 60 comandos integrados** — operaciones de archivos, procesamiento de texto, archivado, redes, scripting y más
- **Banderas cortas combinadas** — `-rf`, `-la`, `-jrc`, etc. funcionan en todos los comandos
- **Lenguajes de scripting** — Go (intérprete MVM), Lua (gopher-lua), JavaScript (goja ES2020+), JSON/YAML (jq/yq), SQLite
- **Sistema de plugins WASM** — extensible con plugins compilados para WASI (runtimes de Python, Ruby, PHP)
- **Plugins nativos de Go** — registra comandos personalizados a través de una interfaz `Plugin` sencilla
- **REPL interactivo** — autocompletado con tab, historial de comandos, script de inicio `.memshrc`
- **Servidor HTTP** — expone el shell sobre HTTP con sistemas de archivos virtuales con alcance por sesión
- **Política de salida de red** — controla la conectividad saliente con listas de permitidos de dominios/CIDR/puertos
- **Uso como biblioteca** — incrusta memsh en programas de Go para scripting de shell seguro y aislado

## Instalación

### Homebrew

```bash
brew tap amjadjibon/memsh
brew install memsh
```

### Instalación con Go

```bash
go install github.com/amjadjibon/memsh@latest
```

### Binarios precompilados

Descarga los binarios precompilados desde la página de [GitHub Releases](https://github.com/amjadjibon/memsh/releases).

**Linux (amd64):**

```bash
curl -Lo memsh.tar.gz https://github.com/amjadjibon/memsh/releases/latest/download/memsh_linux_amd64.tar.gz
tar xzf memsh.tar.gz
sudo mv memsh /usr/local/bin/
```

**macOS (Apple Silicon):**

```bash
curl -Lo memsh.tar.gz https://github.com/amjadjibon/memsh/releases/latest/download/memsh_darwin_arm64.tar.gz
tar xzf memsh.tar.gz
sudo mv memsh /usr/local/bin/
```

**macOS (Intel):**

```bash
curl -Lo memsh.tar.gz https://github.com/amjadjibon/memsh/releases/latest/download/memsh_darwin_amd64.tar.gz
tar xzf memsh.tar.gz
sudo mv memsh /usr/local/bin/
```

**Windows (amd64):**

Descarga [`memsh_windows_amd64.zip`](https://github.com/amjadjibon/memsh/releases/latest/download/memsh_windows_amd64.zip), extráelo y añade `memsh.exe` a tu `PATH`.

### Compilar desde el código fuente

```bash
git clone https://github.com/amjadjibon/memsh.git
cd memsh
go build -o memsh .
```

### Verificar la instalación

```bash
memsh --help
```

## Inicio rápido

```bash
# REPL interactivo
memsh

# Ejecutar un script
memsh ./path/to/script.sh

# Comando con pipe
echo "mkdir /tmp && echo hello > /tmp/f && cat /tmp/f" | memsh

# Servidor HTTP
memsh serve
memsh serve --addr :3000 --session-ttl 1h --cors-origin https://app.example.com

# Shell con red restringida
memsh --net-mode allowlist \
  --net-allow-domain 'httpbin.org' \
  --net-allow-port 443 \
  -c 'curl https://httpbin.org/get'
```

## Uso como biblioteca

```go
package main

import (
    "bytes"
    "context"
    "fmt"
    "log"

    "github.com/amjadjibon/memsh/pkg/shell"
)

func main() {
    ctx := context.Background()

    var out bytes.Buffer
    sh, err := shell.New(shell.WithStdIO(nil, &out, &out))
    if err != nil {
        log.Fatal(err)
    }
    defer sh.Close()

    err = sh.Run(ctx, `
        mkdir -p /home/user/docs
        echo '{"name":"alice","role":"admin"}' > /home/user/docs/user.json
        jq -r .name /home/user/docs/user.json
    `)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Print(out.String()) // alice
}
```

### Inicialización previa del sistema de archivos virtual

```go
fs := afero.NewMemMapFs()
afero.WriteFile(fs, "/config.yaml", []byte("host: localhost\nport: 8080\n"), 0644)

var out bytes.Buffer
sh, _ := shell.New(
    shell.WithFS(fs),
    shell.WithStdIO(nil, &out, &out),
)
sh.Run(ctx, "yq .host /config.yaml") // localhost
```

## Comandos

| Comando | Descripción |
| --- | --- |
| `cat` | Concatena e imprime archivos |
| `cd` | Cambia el directorio de trabajo |
| `chmod` | Cambia permisos de archivos (`-R` recursivo) |
| `cp` | Copia archivos o directorios (`-r`) |
| `cut` | Extrae campos (`-f`) o caracteres (`-c`) |
| `date` | Imprime la fecha y hora actuales |
| `df` | Informa el uso de espacio en disco del sistema de archivos |
| `diff` | Compara dos archivos línea por línea (`-u` unificado) |
| `du` | Estima el uso de espacio de archivos |
| `echo` | Imprime argumentos (`-n`, `-e`) |
| `env` | Imprime o establece variables de entorno |
| `find` | Busca en el sistema de archivos virtual (`-name`, `-type`, `-maxdepth`) |
| `grep` | Busca en el contenido de archivos (`-i`, `-n`, `-v`, `-r`, `-c`, `-l`, `-w`, `-o`) |
| `head` | Imprime las primeras N líneas (`-n`) o bytes (`-c`) |
| `ln` | Crea enlaces duros o simbólicos (`-s`, `-f`) |
| `ls` | Lista el contenido de directorios (`-l`, `-a`, `-R`) |
| `mkdir` | Crea directorios (`-p`, `-v`, `-m`) |
| `mv` | Mueve o renombra archivos |
| `printf` | Formatea e imprime datos |
| `pwd` | Imprime el directorio de trabajo |
| `read` | Lee una línea desde stdin hacia una variable |
| `rm` | Elimina archivos o directorios (`-f`, `-r`, `-v`) |
| `rmdir` | Elimina directorios vacíos |
| `sed` | Editor de flujo (sustitución) |
| `seq` | Imprime una secuencia de números |
| `sleep` | Espera una cantidad de tiempo especificada |
| `sort` | Ordena líneas (`-r`, `-u`, `-n`) |
| `stat` | Muestra el estado de un archivo |
| `tail` | Imprime las últimas N líneas (`-n`) o bytes (`-c`) |
| `tee` | Lee stdin; escribe en stdout y archivos (`-a`) |
| `timeout` | Ejecuta un comando con un límite de tiempo |
| `touch` | Crea o actualiza marcas de tiempo de archivos |
| `tr` | Traduce o elimina caracteres (`-d`, `-s`, `-c`) |
| `uniq` | Filtra líneas duplicadas adyacentes (`-c`, `-d`, `-u`) |
| `wc` | Cuenta líneas, palabras y bytes (`-l`, `-w`, `-c`) |
| `which` | Ubica un comando |
| `xargs` | Construye y ejecuta líneas de comando desde stdin |
| `yes` | Repite la salida de una cadena |
| `awk` | Escaneo y procesamiento de patrones |
| `base64` | Codifica o decodifica base64 (`-d`) |
| `bc`, `expr` | Calculadora de precisión arbitraria / evaluador de expresiones |
| `column` | Formatea salida en columnas |
| `crontab` | Programa comandos con expresiones cron |
| `curl` | Transfiere datos desde URLs |
| `envsubst` | Sustituye variables de entorno en cadenas |
| `go` | Herramienta Go — `go run`, `go test`, `go fmt` contra el sistema de archivos virtual |
| `goja` | Ejecuta código JavaScript (ES2020+) |
| `git` | Implementación pura en Go de git |
| `gzip`, `gunzip` | Comprime/decomprime archivos gzip |
| `hexdump`, `xxd` | Volcado hexadecimal de archivos |
| `jq` | Procesador de JSON desde línea de comandos |
| `less`, `more` | Visor paginable con scroll (terminal web) |
| `ln` | Crea enlaces |
| `lua` | Ejecuta código Lua 5.1 |
| `man`, `help` | Muestra ayuda para comandos |
| `md5sum`, `sha256sum`, … | Suma de verificación de archivos (md5, sha1, sha224, sha256, sha384, sha512) |
| `mktemp` | Crea un archivo o directorio temporal |
| `sqlite3` | Shell de base de datos SQLite |
| `ssh` | Conecta a un servidor memsh remoto |
| `tar` | Archiva archivos |
| `tree` | Muestra el contenido de directorios como un árbol ASCII (`-L`, `-a`, `-d`, `-f`) |
| `tput`, `stty` | Estubs de control de terminal |
| `yq` | Procesador de YAML/JSON desde línea de comandos |
| `zip`, `unzip` | Comprime/decomprime archivos zip |

## Sistema de plugins

### Plugins nativos de Go

Implementa la interfaz `Plugin` desde `pkg/shell/plugins`:

```go
import (
    "github.com/amjadjibon/memsh/pkg/shell/plugins"
    "mvdan.cc/sh/v3/interp"
)

type HelloPlugin struct{}

func (HelloPlugin) Name() string        { return "hello" }
func (HelloPlugin) Description() string { return "greet the user" }
func (HelloPlugin) Usage() string       { return "hello [name]" }

func (HelloPlugin) Run(ctx context.Context, args []string) error {
    hc := interp.HandlerCtx(ctx)  // E/S consciente de pipes: siempre úsalo
    sc := plugins.ShellCtx(ctx)   // FS virtual, cwd, ResolvePath, SetEnv, …
    fmt.Fprintf(hc.Stdout, "Hello from %s!\n", sc.Cwd)
    return nil
}
```

Regístralo en el momento de creación del shell:

```go
sh, _ := shell.New(shell.WithPlugin(HelloPlugin{}))
```

O agrégalos a `defaultNativePlugins()` en `pkg/shell/defaults.go` para incluirlo en cada instancia del shell.

### Procesamiento JSON (`jq`)

```bash
echo '{"name":"alice","scores":[10,20,30]}' | jq .name          # "alice"
echo '{"name":"alice"}' | jq -r .name                           # alice (sin comillas)
echo '{"items":[1,2,3]}' | jq '.items | length'                 # 3
jq -n '{generated: true}'                                        # entrada nula
jq -rc .name data.json                                           # banderas combinadas
```

### Procesamiento YAML/JSON (`yq`)

```bash
echo 'name: alice' | yq .name                                    # alice
echo 'name: alice' | yq -j .                                     # salida JSON
printf 'items:\n  - a\n  - b\n' | yq '.items[0]'                # a
yq .host /config.yaml                                            # lee desde el FS virtual
yq -jc . data.yaml                                               # salida JSON compacta
```

### Scripting en Lua

```bash
lua -e 'print("hello from lua")'
echo 'for i=1,3 do print(i) end' | lua
lua /script.lua
```

### Scripting en Go (`go`)

El comando `go` emula la cadena de herramientas de Go contra el sistema de archivos virtual, respaldado por el [intérprete MVM](https://github.com/mvm-sh/mvm). La stdlib se importa automáticamente: no se necesitan declaraciones `import` para expresiones en línea.

```bash
# go run — ejecuta un archivo fuente
echo 'package main' > /main.go
echo 'func main() { fmt.Println("hello") }' >> /main.go
go run /main.go                                    # hello

# go run — fibonacci
echo 'package main' > /fib.go
echo 'func fib(n int) int { if n<=1{return n}; return fib(n-1)+fib(n-2) }' >> /fib.go
echo 'func main() { for i:=0;i<=7;i++ { fmt.Println(i,fib(i)) } }' >> /fib.go
go run /fib.go

# go test — ejecuta funciones Test*; informa PASS/FAIL por prueba
echo 'package main' > /math_test.go
echo 'import "testing"' >> /math_test.go
echo 'func TestAdd(t *testing.T) { if 1+1!=2 { t.Error("broken") } }' >> /math_test.go
go test /                                          # --- PASS: TestAdd / ok
go test ./...                                      # recorre todos los subdirectorios
go test / -run TestAdd                             # filtra por regex de nombre
go test / -v                                       # detallado (líneas === RUN)

# go fmt — formatea archivos fuente de gofmt en el FS virtual
echo 'package main' > /ugly.go
echo 'func main(){fmt.Println("hi")}' >> /ugly.go
go fmt /ugly.go
cat /ugly.go                                       # correctamente formateado

# pipe desde stdin — stdlib importada automáticamente, sin paquete/import
echo 'fmt.Println(strings.ToUpper("hello"))' | go
echo 'fmt.Println(math.Sqrt(144))' | go

# go version
go version                                         # go version mvm0.3.0
```

**Notas:**
- `go test` reescribe `*testing.T` hacia un shim integrado; `t.Error`, `t.Errorf`, `t.Fatal`, `t.Fatalf`, `t.Log`, `t.Run`, `t.Skip` funcionan todos.
- MVM está en fase alpha (v0.3.0); algunos paquetes de la stdlib tienen soporte parcial. Las llamadas no soportadas se manifiestan como errores del intérprete.
- El modo stdin y `go run` usan importación automática de la stdlib: `fmt`, `strings`, `math`, etc. funcionan sin imports explícitos.

### Scripting en JavaScript

```bash
goja -e 'console.log("hello")'
echo 'console.log("test")' | goja
goja /script.js
goja -e 'const arr=[1,2,3]; console.log(arr.map(x=>x*2).join(","))'
```

### Plugins WASM

Los plugins WASM se compilan con `GOOS=wasip1 GOARCH=wasm`. El FS virtual se monta vía WASI, por lo que la E/S de archivos va directamente a `afero.MemMapFs`.

```bash
go run . plugin install python   # Python 3.12.0 (~25 MB)
go run . plugin install ruby     # Ruby 3.2.2 slim (~8 MB)
go run . plugin install php      # PHP 8.2.6 slim (~6 MB)
go run . plugin install /path/to/plugin.wasm  # archivo local
go run . plugin list             # lista plugins instalados
```

Los runtimes instalados se almacenan en `~/.memsh/plugins/*.wasm`.

### Prioridad de carga de plugins

1. Opciones `WithPlugin(p)` o `WithPluginBytes(name, wasm)`
2. Plugins nativos de Go registrados en `defaultNativePlugins()`
3. WASM incrustado desde el mapa `defaultPlugins` (actualmente vacío)
4. `/memsh/plugins/*.wasm` en el FS virtual
5. `~/.memsh/plugins/*.wasm` en el sistema de archivos real del SO

## Opciones

| Opción | Descripción |
| --- | --- |
| `WithFS(fs)` | Establece el sistema de archivos afero (predeterminado: `afero.NewMemMapFs()`) |
| `WithCwd(path)` | Establece el directorio de trabajo inicial |
| `WithEnv(env)` | Establece las variables de entorno iniciales |
| `WithStdIO(in, out, err)` | Establece los flujos estándar de E/S |
| `WithPlugin(p)` | Registra un plugin nativo |
| `WithBuiltin(name, fn)` | Registra una función raw como comando |
| `WithPluginBytes(name, wasm)` | Registra un plugin WASM desde bytes |
| `WithWASMEnabled(bool)` | Habilita/deshabilita el runtime WASM (predeterminado: true) |
| `WithPluginFilter(names)` | Lista de permitidos para descubrimiento de plugins WASM |
| `WithDisabledPlugins(names...)` | Excluye plugins específicos por nombre |
| `WithAllowExternalCommands(bool)` | Permite caer en ejecutables reales del SO (predeterminado: false) |
| `WithInheritEnv(bool)` | Hereda el entorno del proceso padre (predeterminado: true; usa false en modo servidor) |
| `WithAliases(map)` | Inicializa la tabla de alias |
| `WithNetworkPolicy(policy)` | Establece política de red saliente (`off`, `allowlist`, `full`) |
| `WithNetworkLimits(limits)` | Establece límites de solicitudes/bytes/tiempo de ejecución de red |
| `WithNetworkUsage(usage)` | Semilla el uso acumulado de red (para sesiones restauradas) |

## Servidor HTTP

```bash
go run . serve                          # escucha en :8080
go run . serve --addr :3000 --cors-origin https://app.example.com
go run . serve --session-ttl 1h --timeout 30s
```

Las sesiones siempre están habilitadas. Envía `X-Session-ID: <id>` en `POST /run` para persistir el sistema de archivos virtual entre solicitudes.

| Endpoint | Descripción |
| --- | --- |
| `GET /` | Interfaz de terminal web |
| `POST /run` | `{"script":"..."}` → `{"output":"...","cwd":"...","error":"..."}` |
| `GET /sessions` | Lista sesiones activas (cwd, marcas de tiempo, contadores de uso de runtime/red) |
| `DELETE /session/{id}` | Destruye una sesión |
| `GET /health` | `{"status":"ok","uptime":"...","sessions":N}` |
| `POST /complete` | `{"input":"...","cursor":N}` → autocompletado con tab |
| `GET /session/{id}/snapshot` | Exporta el sistema de archivos de la sesión como JSON |
| `POST /session/{id}/snapshot` | Importa un snapshot (usa `"new"` como id para crear) |

### Banderas de política de red

Estas banderas funcionan tanto para `memsh` local como para `memsh serve`:

```bash
--net-mode off|allowlist|full
--net-allow-domain <domain>    # repetible, soporta *.example.com
--net-allow-cidr <cidr>        # repetible, ej. 203.0.113.0/24
--net-allow-port <port>        # repetible, ej. 443
--net-max-requests <n>         # 0 = ilimitado
--net-max-bytes-sent <n>       # 0 = ilimitado
--net-max-bytes-recv <n>       # 0 = ilimitado
--net-max-runtime <duration>   # 0 = ilimitado, ej. 30s
```

Ejemplos:

```bash
# Bloquea toda la red saliente
memsh --net-mode off -c 'curl https://example.com'

# Permite solo HTTPS a httpbin.org
memsh --net-mode allowlist \
  --net-allow-domain 'httpbin.org' \
  --net-allow-port 443 \
  -c 'curl https://httpbin.org/get'
```

Si DNS falla (`lookup <host>: no such host`), eso es resolución de entorno/red, no un bloqueo de política. Un bloqueo de política devuelve errores explícitos como `network disabled by policy` o `destination port ... is not allowed`.

`GET /sessions` ahora incluye contadores de uso:

```json
{
  "id": "abc123",
  "cwd": "/",
  "created_at": "2026-04-17T08:00:00Z",
  "last_use": "2026-04-17T08:01:00Z",
  "runtime_ms": 1420,
  "network_requests": 3,
  "network_bytes_sent": 512,
  "network_bytes_received": 4096,
  "network_runtime_ms": 280
}
```

## Integración con LLM

memsh tiene dos modos para conectar LLMs: un **servidor MCP** (compatible con cualquier cliente MCP) y un **agente** integrado (bucle interactivo ReAct).

### Servidor MCP (`memsh mcp`)

El servidor MCP expone una sola herramienta `memsh` que permite a cualquier LLM ejecutar comandos bash en un sistema de archivos virtual aislado en memoria. El sistema operativo real nunca se toca.

**Transportes:**

| Transporte | Comando | Caso de uso |
|---|---|---|
| stdio (predeterminado) | `memsh mcp` | Claude Desktop, Claude Code CLI |
| HTTP (MCP 2025-03-26+) | `memsh mcp --transport http --addr :8080` | Programático / multi-sesión |
| SSE (legacy) | `memsh mcp --transport sse --addr :8080` | Clientes MCP legacy |

**Claude Desktop** (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "memsh": {
      "command": "/usr/local/bin/memsh",
      "args": ["mcp"]
    }
  }
}
```

**Claude Code CLI:**

```bash
claude mcp add memsh -- memsh mcp
```

**Otros clientes MCP** — inicia el transporte HTTP y apunta tu cliente al endpoint:

```bash
memsh mcp --transport http --addr :8080
# Conecta a: http://localhost:8080/
```

**Comportamiento de la herramienta:**
- Nombre de la herramienta: `memsh`, campo de entrada: `command` (string)
- El sistema de archivos virtual **persiste entre llamadas** dentro de una sesión: úsalo como bloc de notas
- `exit` / `quit` se tratan como éxito, no como errores
- stdin no está disponible; los comandos que leen stdin reciben EOF
- Devuelve la salida del comando + directorio de trabajo actual (`Cwd: /ruta`)
- Timeout por llamada (predeterminado 30 s, mínimo 5 s): `memsh mcp --timeout 1m`
- Plugins WASM (Python/Ruby/PHP) deshabilitados por defecto para un arranque rápido: `memsh mcp --wasm`

**Ejemplo de resultado de llamada a la herramienta:**
```
/home/user/data
file1.txt  file2.txt

Cwd: /home/user/data
```

## Configuración

`~/.memsh/config.toml` se carga al inicio (archivo ausente = valores predeterminados):

```toml
[shell]
wasm = true          # establece false para omitir toda carga WASM (arranque más rápido)

[plugins]
wasm    = ["python"] # lista de permitidos de nombres de ~/.memsh/plugins/*.wasm; vacío = cargar todos
disable = ["wc"]     # excluye plugins específicos por nombre (nativos o WASM)
```

Archivos de configuración:

- `~/.memsh/config.toml` — configuración del shell y plugins
- `~/.memsh/.memshrc` — script de inicio (ejecutado al iniciar el REPL y la primera sesión HTTP)
- `~/.memsh/history/` — historial de comandos por sesión
- `~/.memsh/plugins/` — plugins WASM instalados por el usuario

## Pruebas

```bash
go test ./...                        # suite completa de pruebas
go test ./tests -v                   # pruebas de integración detalladas
go test ./tests -run TestJq -v       # suite única
go test ./pkg/shell/... -run TestName  # pruebas del paquete shell
```

## Desarrollo

```bash
# Compilar
make build

# Ejecutar pruebas
make test

# Ejecutar reporte de cobertura
make cover

# Linter
make lint

# Limpiar artefactos de compilación
make clean

# Ver todos los comandos disponibles
make help
```

### Crear una versión (release)

El proyecto usa [GoReleaser](https://goreleaser.com/) para versiones automatizadas y generación del cask de Homebrew.

```bash
# 1. Probar el proceso de release (dry-run)
make release-dry-run TAG=v1.0.0

# 2. Crear la release real
make release TAG=v1.0.0
```

El comando `make release` realizará:

1. **Commit y push de cambios sin comprometer** (prepara para la release)
2. **Limpia el directorio `dist/`** (elimina artefactos de compilación antiguos)
3. Limpia artefactos de compilación (bin/ y archivos *.wasm)
4. Crea y hace push de una tag de git
5. Compila binarios para todas las plataformas (Linux, macOS, Windows × AMD64, ARM64)
6. Crea una GitHub Release con todos los binarios
7. **Genera y empuja el cask de Homebrew** automáticamente vía goreleaser a [`homebrew-memsh`](https://github.com/amjadjibon/homebrew-memsh)

Después de la release, los usuarios pueden instalarlo mediante:

```bash
brew tap amjadjibon/memsh
brew install memsh
```

**Nota:** Asegúrate de que `GITHUB_TOKEN` esté configurado para que goreleaser pueda crear releases y hacer push a los repositorios.

## Requisitos

- Go 1.26+

## Licencia

Consulta [LICENSE](LICENSE).
