# Slice 12: authenticated Java access to the Clyde world

Goal: Lachlan can join the existing Clyde Paper world from Java Edition,
through authenticated Clyde-only proxies like Bonnie's, while preserving the
world, player identities, bot memory and existing Bedrock entrance.

## Done when

- Record the actual host service/config/artifact baseline, Java client version
  and approved online UUID, existing whitelist/playerdata and Floodgate linking
  policy. Do not mistake a Bedrock-only endpoint for a failed Java service.
  Ask only for the owner's Java username/version, never credentials or tokens.
- Add two Clyde-only Velocity services on `minecraft_clyde_net`: public
  `minecraft-clyde-proxy` authenticates Java accounts (`online-mode=true`);
  private `minecraft-clyde-bot-proxy` permits the offline bot and publishes
  absolutely no host port. Both use MODERN forwarding to the same private
  Paper backend. No changes to Bonnie, its proxies or unrelated host services.
- Public Java is `minecraft.loxflix.xyz:25567`, with only new TCP25567 allowed
  as required. Backend Java/RCON remain unpublished. Paper stays offline but
  enables Velocity forwarding with `proxies.velocity.online-mode=true`.
  A fresh independent Clyde-only secret reaches Paper and both proxies via
  the verified environment overrides, never logs/Git/models. Preserve a
  provisioned nonempty value; no reuse of Bonnie's secret.
- Repoint only Clyde's Minecraft host to its private bot proxy. Preserve its
  accepted image, offline UUID, direct private RCON/credential, whole memory
  volume, models, skills, safety and budgets. Do not implement a custom body
  forwarding handshake or expose the offline proxy even on Tailscale.
- Move Geyser/Floodgate to the new public proxy, retaining native HTTPS/
  NetherNet on TCP+UDP19134, certificate/key, signaling identity, prefix,
  space and linking policy. Inventory/migrate any local-link data before
  admitting players. Remove only managed backend Geyser/Floodgate jars from
  the active plugin set; back up their data. Keep ViaVersion/ViaBackwards on
  Paper. No world conversion, playerdata renaming or fabricated UUID mapping.
- Reuse the actual pinned versions/artifacts below, with recorded checksums
  for matching Velocity plugins. Validate Compose plus effective settings,
  mounts, network membership, secrets equality and public/private listeners.
  Fresh secret provisioning must not break unrelated Compose operations.
- Run the original proxy matrix on disposable infrastructure with the actual
  artifacts: direct-backend rejection, public offline-auth rejection,
  wrong-secret rejection, whitelist rejection, private Clyde spawn with its
  exact offline UUID, and bot26.1 translated by Via on Paper26.2. A disposable
  forwarded-UUID fixture must select preexisting inventory/position; never
  install the UUID-override test plugin live or call it account-auth proof.
  Verify public Java status and certificate-trusted Bedrock signaling,
  preserving the advertised UDP endpoint. Signaling is not Bedrock gameplay.
- Rehearse rollback including persisted Paper forwarding settings, backend
  plugins, Bedrock identities/ports and bot route. Removing a Compose patch
  alone does not disable persisted Velocity support. Prefer retaining same-UUID
  world progress; restoring a backup loses play since the snapshot. Never
  downgrade the bot against migrated memory without its matching snapshot.
- Before cutover, confirm no human is playing, stop Clyde then only its Paper,
  and take/verify consistent whole-world/data and whole-memory backups.
  Preserve private inputs and unrelated dirty checkout work. Apply the merged
  infrastructure PR with targeted startup only; never start an entire profile
  that could include Bonnie services. Verify bot UUID/inventory/position,
  memory continuity and unchanged non-Clyde container baselines afterwards.
- Real authenticated owner Java login and retained player identity/inventory/
  position are the client acceptance gate; also check preserved real Bedrock
  login when a client is available. If owner/client access is unavailable,
  report client acceptance as pending, not passed by synthetic/offline tests.
  Observe disconnect/restart/resource health without speculative tuning.
- Use `lox/selfhosted-ovh` Auto-review, PR-description and Babysit flow; fix
  material review findings, merge when green, then perform the authorized
  scoped rollout. Update its runbook with backup-before-recreation, access,
  reliable stop and rollback commands. Report exact versions/image/PR,
  verification, preserved data and outstanding client checks to coordinator;
  keep this plan and roadmap current with actual outcome.

## Starting point

The world already runs Paper26.2 build134, with ViaVersion/ViaBackwards5.12.1,
private offline Java/RCON and backend Geyser/Floodgate. Human access currently
exposes only Bedrock TCP+UDP19134. Clyde source `7700696` is live with full
Sol6.1-low planner/executor, Jev and persistent `clyde-play-memory`; its direct
backend route must change when modern forwarding becomes mandatory.

The existing host owner and infrastructure PR-author threads retain ownership;
do not create competing host checkouts or a parallel infrastructure slice.
Implementation belongs in `lox/selfhosted-ovh` Compose, Clyde-specific proxy/
Paper/plugin configuration and `docs/minecraft-clyde.md`, not this bot repo.
Reuse Bonnie PR6's produced/tested method and
[original thread](https://ampcode.com/threads/T-01a14703-1539-704a-bbed-daa2439338bf),
not Bonnie's live services, secret, identity or network.

## Notes

- Host proposal verified Velocity4.2.0 build30 and mc-proxy digest
  `sha256:2a77bff989e122d4c3ff3c6689601628e757ddbb1fd9436676cbeac6e7ae0233`.
  Keep existing Paper/Via pins; use matching Geyser2.12.0 build1265 and
  Floodgate2.2.5 build141 Velocity artifacts with independently recorded hashes.
  No opportunistic version/model/resource upgrades.
- Paper uses `PAPER_VELOCITY_SECRET`; proxies use
  `VELOCITY_FORWARDING_SECRET`. Paper persists its forwarding settings/secret.
  Generate/provision `CLYDE_VELOCITY_SECRET` privately with mode0600 handling.
- Offline `Clyde` UUID is `afbfda40-9cec-3636-b8bf-8b10c9fad526`.
  Preserve all six current whitelist entries. Do not run `whitelist add Clyde`:
  account lookup can select an unrelated authenticated account. Any missing
  owner allowlist entry must use the verified owner online identity explicitly.
- Current Floodgate prefix is `.`, replace-spaces=true, linking enabled with
  own-linking disabled; inspect actual local files anyway. Preserve existing
  Geyser NetherNet identity and copied TLS files with current permissions.
- Brief Clyde-world downtime is necessary and authorized by the requested
  topology change. Bonnie and unrelated services must remain uninterrupted.
  Existing reliable bot stop is `docker compose stop clyde` on the host.

## Not in this slice

New world, Bonnie deployment/changes, DNS changes, public offline Java or RCON,
new bot image, planner latency tuning, authentication code in the body,
automatic skill promotion, unrelated host cleanup or broad gameplay guarantees.
