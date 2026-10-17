# go-mclib spike (not the Clyde body)

Research code only. Findings: `docs/plans/spike-go-mclib.md` on `main`.
No HTTP API, safety boundary, general collect implementation, or production
readiness is claimed. All joins here are offline mode, direct to local Paper.

## Run

Requires Go 1.27 (the harness RCON helper), Java 25, curl, jq, patch;
use the repo's installed Go and JDK. Upstream go-mclib requires Go ≥1.25.
Start a **separate disposable** Paper 26.2 server, not the gym:

```sh
mkdir -p "$HOME/paper-spike"
# Download Paper 26.2 build 134 (the inspected build).
curl -fL -o "$HOME/paper-spike/paper.jar" https://fill-data.papermc.io/v1/objects/3cce0a4922baf79bd8e1275ee5ff9f073f24675770e09b0d4be9442d513e7a6b/paper-26.2-134.jar
echo eula=true > "$HOME/paper-spike/eula.txt"
sed -e 's/server-port=25565/server-port=25566/' \
    -e 's/rcon.port=25575/rcon.port=25576/' \
    -e 's/^rcon.password=.*/rcon.password=spike/' \
    gym/server.properties > "$HOME/paper-spike/server.properties"
amp orb service start paper-spike --command "cd $HOME/paper-spike && $HOME/tools/jdk/bin/java -Xms1G -Xmx2G -jar paper.jar --nogui"
# Wait for Done in amp orb service logs paper-spike.
cd spike/gomclib
./prepare
go test ./...
go run . -fixture -walks 4 -wall
```

`-fixture` **modifies only that disposable server** over RCON: peaceful/day,
force-loads an arena, removes item entities, fills a platform at Y=160,
sets a log, clears Clyde's inventory, and teleports it to the start. Normal
survival movement, bare-hand digging, and pickup then use client packets.
The final log count is checked independently with RCON `clear … logs 0`.
No teleport is used to satisfy the walking or collecting steps.

`-wall` adds a separate wall-contact probe after the main script. It resets
the correction counter after its setup teleport, then pushes east into a
stone wall for 2 s and walks south along it for 3 s. Its `ok` means the probe
ran, **not** that physics is correct: inspect its correction count. The final
report's correction count is wall-only when `-wall` is set; without it,
it covers the whole script after spawn/setup. `walkedMetres` sums straight-line
displacements of completed navigations, not integrated path length.

Compare the mineflayer-style width workaround:

```sh
./prepare wide                  # optional physics width 0.6 -> 0.6002
go run . -fixture -walks 0 -wall
./prepare                       # restore unmodified client; keep NBT fix
go build -race -o /tmp/gomclib-race .
/tmp/gomclib-race -fixture -walks 0 -wall  # exits 66; upstream data races
```

Source pins are in go.mod. All three upstream repositories returned 404 on
2026-10-17, but the public Go module proxy served the pinned source archives.
`prepare` copies the data module to ignored `forks/data`, preserving its MIT
license, and applies `data-nbt-framing.patch`. The optional client copy also
preserves its MIT license. It never edits the shared module cache.
The patch fixes six NBT readers; only registry decoding was required/tested
for joining. Other packet writers are not repaired. This is not a full fork.

The spike supplies bare-hand timing and a small collect loop; these are **not
upstream high-level APIs**. Only harvestable-without-tool blocks are valid
for this prototype. It waits for an authoritative air update and checks item
gain. Initial pickup attempts chased dirt or chose the top of a floating
log; the final fixture has one log, filters drops by item metadata and waits
800 ms for them to settle. This is deliberately not a robust tree harvester.

`SPIKE_DUMP=/existing/dir` saves configuration packets for debugging.
`SPIKE_VERBOSE=1` enables upstream verbose logging. A memory watchdog exits
with a goroutine dump if the unmodified registry parser allocates excessively;
it is diagnostic, not a reliable resource limit. Use an OS limit for untrusted
servers. The original parser exhausted the orb's memory before the patch.

`results/` contains observed reports/transcripts. Race output is a failure,
not a green check. Natural-terrain failure predates fixture controls; the
controlled wall A/B is the useful physics comparison. `amp orb service stop
paper-spike` stops the server when finished.
