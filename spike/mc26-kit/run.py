"""Replay the mc26-kit research fixture, not a Clyde acceptance suite."""
import argparse
import json
import pathlib
import queue
import re
import socket
import struct
import subprocess
import sys
import threading
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('label', choices=['baseline-repeat', 'wide', 'wide-contained'])
parser.add_argument('--binary', type=pathlib.Path)
parser.add_argument('--server-dir', type=pathlib.Path,
                    default=pathlib.Path.home() / 'paper-spike')
parser.add_argument('--output', type=pathlib.Path)
parser.add_argument('--confirm-disposable', action='store_true', required=True,
                    help='acknowledge that this resets the fixture area and bot inventory')
args = parser.parse_args()
ROOT = pathlib.Path(__file__).resolve().parent
label = args.label
binary = str((args.binary or ROOT / 'bin' / (
    'robot-default' if label == 'baseline-repeat' else 'robot-wide')).resolve())
OUT = args.output or ROOT / 'runs' / label
OUT.mkdir(parents=True, exist_ok=True)
properties = dict(line.split('=', 1) for line in (
    args.server_dir / 'server.properties').read_text().splitlines()
    if line and not line.startswith('#') and '=' in line)
if (properties.get('server-ip') != '127.0.0.1'
        or properties.get('server-port') != '25566'
        or properties.get('rcon.port') != '25576'):
    parser.error('requires the disposable loopback server on ports 25566/25576')
password = properties['rcon.password'].strip()
steps = []
transcript = (OUT / 'transcript.log').open('w')


def record(text):
    print(text, flush=True)
    transcript.write(text + '\n')
    transcript.flush()


def receive(conn):
    def exact(n):
        data = b''
        while len(data) < n:
            chunk = conn.recv(n - len(data))
            if not chunk:
                raise RuntimeError('RCON closed')
            data += chunk
        return data
    return exact(struct.unpack('<i', exact(4))[0])


def rcon(command):
    with socket.create_connection(('127.0.0.1', 25576), timeout=10) as conn:
        for kind, text in [(3, password), (2, command)]:
            body = struct.pack('<ii', 27, kind) + text.encode() + b'\0\0'
            conn.sendall(struct.pack('<i', len(body)) + body)
            answer = receive(conn)
            if kind == 3:
                if struct.unpack('<i', answer[:4])[0] == -1:
                    raise RuntimeError('RCON authentication failed')
            else:
                result = answer[8:-2].decode(errors='replace')
    record(f'RCON {command} => {result}')
    return result


for command in ['difficulty peaceful', 'time set day',
                'forceload add -32 -32 48 48']:
    rcon(command)
time.sleep(3)
for command in ['kill @e[type=item]',
                'fill -32 160 -32 48 160 48 grass_block',
                'fill -32 161 -32 48 164 48 air',
                'setblock 17 161 8 oak_log', 'setworldspawn 0 161 0']:
    rcon(command)

lines = queue.Queue()
start = time.monotonic()
proc = subprocess.Popen([binary, '-address',
                         '127.0.0.1:25566', '-name', 'KitSpike'],
                        stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                        stderr=subprocess.STDOUT, text=True, bufsize=1)


def read_output():
    for line in proc.stdout:
        line = line.rstrip()
        record(line)
        lines.put(line)


thread = threading.Thread(target=read_output, daemon=True)
thread.start()


def send(command):
    record('SEND ' + command)
    proc.stdin.write(command + '\n')
    proc.stdin.flush()


def wait_for(pattern, timeout=30):
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        try:
            line = lines.get(timeout=min(1, max(.01, end-time.monotonic())))
        except queue.Empty:
            if proc.poll() is not None:
                raise RuntimeError(f'robot exited {proc.returncode}')
            continue
        if re.search(pattern, line):
            return line
    raise TimeoutError(pattern)


def bot(command, timeout=30):
    name = command.split()[0]
    before = time.monotonic()
    send(command)
    answer = wait_for(r'robot: (?:error ' + name + r':|' + name + r' )', timeout)
    result = {'command': command, 'seconds': round(time.monotonic()-before, 3),
              'answer': answer, 'ok': 'robot: error ' not in answer}
    steps.append(result)
    return answer


try:
    wait_for('Login success', 40)
    record(f'JOIN_SECONDS {time.monotonic()-start:.3f}')
    time.sleep(3)
    rcon('clear KitSpike')
    rcon('gamemode survival KitSpike')
    rcon('tp KitSpike 0.5 161 0.5')
    time.sleep(1)
    bot('state')
    bot('goto 12 161 8')
    rcon('data get entity KitSpike Pos')
    bot('dig 13 160 8')
    rcon('execute if block 13 160 8 air run say DIG_CONFIRMED')
    bot('goto 16 161 8')
    bot('dig 17 161 8')
    bot('collect 8')
    bot('inv')
    rcon('clear KitSpike minecraft:oak_log 0')
    send('say mc26-kit Paper smoke: walked, dug and collected')
    time.sleep(.5)
    for goal in ['0 161 8', '0 161 -12', '20 161 -12', '20 161 8']:
        bot('goto ' + goal)
    bot('state')

    # Match the go-mclib wall probe; commanded setup teleport is excluded.
    rcon('fill 20 161 -4 20 163 20 stone')
    rcon('tp KitSpike 17.5 161 0.5')
    time.sleep(1)
    bot('look -90 0')
    wall_before = bot('state')
    bot('keys forward 40')
    bot('look 0 0')
    bot('keys forward 60')
    wall_after = bot('state')
    before_n = int(re.search(r'teleports=(\d+)', wall_before)[1])
    after_n = int(re.search(r'teleports=(\d+)', wall_after)[1])
    record(f'WALL_CORRECTIONS {after_n-before_n}')
    rcon('data get entity KitSpike Pos')
    rcon('fill 20 161 -4 20 163 20 air')

    bot('recipe oak_planks')
    bot('craft oak_planks 4')
    rcon('clear KitSpike minecraft:oak_planks 0')
    bot('craft crafting_table 1')
    inv = bot('inv')
    slot = re.search(r'(\d+)=minecraft:crafting_table\*', inv)
    if slot is None or int(slot[1]) > 8:
        raise RuntimeError('crafting table not found in hotbar: ' + inv)
    bot('hold ' + slot[1])
    bot('goto 17 161 8')
    bot('place 18 161 8')
    rcon('execute if block 18 161 8 crafting_table run say TABLE_CONFIRMED')
    # Seed precisely the ingredients; the client must perform the grid clicks.
    rcon('give KitSpike oak_planks 3')
    rcon('give KitSpike stick 2')
    time.sleep(.5)
    bot('open 18 161 8')
    bot('craft wooden_pickaxe 1')
    bot('close')
    rcon('clear KitSpike minecraft:wooden_pickaxe 0')
    rcon('setblock 17 161 11 chest')
    bot('goto 17 161 10')
    bot('open 17 161 11')
    bot('store wooden_pickaxe')
    rcon('data get block 17 161 11 Items')
    bot('take wooden_pickaxe')
    bot('close')
    rcon('clear KitSpike minecraft:wooden_pickaxe 0')
    # Stop a walk in flight, then immediately give another goal.
    send('goto 45 161 40')
    time.sleep(.6)
    bot('stop')
    wait_for(r'robot: error goto: stopped', 5)
    bot('goto 12 161 12')
    rcon('data get entity KitSpike Pos')
    time.sleep(1)
    bot('state')

    # A wall that forces a detour rather than a straight-line movement test.
    rcon('fill 4 161 20 4 163 30 stone')
    rcon('tp KitSpike 0.5 161 25.5')
    time.sleep(1)
    bot('goto 9 161 25')
    rcon('data get entity KitSpike Pos')
    bot('state')
    # Three one-block rises and a three-block drop back to the grass platform.
    for command in ['fill 0 161 35 0 161 37 stone',
                    'fill 1 161 35 1 162 37 stone',
                    'fill 2 161 35 3 163 37 stone',
                    'tp KitSpike -0.5 161 36.5']:
        rcon(command)
    time.sleep(1)
    bot('goto 3 164 36')
    rcon('data get entity KitSpike Pos')
    bot('goto -1 161 36')
    rcon('data get entity KitSpike Pos')
    bot('state')

    if label == 'wide-contained':
        # One-deep water is contained below the surrounding grass surface.
        for command in ['fill 25 161 24 36 163 24 stone',
                        'fill 25 161 26 36 163 26 stone',
                        'fill 26 159 25 35 159 25 stone',
                        'fill 26 160 25 35 160 25 water',
                        'tp KitSpike 25.5 161 25.5']:
            rcon(command)
        time.sleep(1)
        bot('goto 36 161 25', 25)
        rcon('data get entity KitSpike Pos')
        bot('state')
        # Unlike a walk, does stop actually abort an in-flight dig?
        rcon('setblock 12 161 13 oak_log')
        rcon('tp KitSpike 12.5 161 12.5')
        time.sleep(1)
        bot('hold 7')
        send('dig 12 161 13')
        time.sleep(.25)
        bot('stop')
        send('goto 12 161 14')
        try:
            answer = wait_for(r'robot: (?:dig |error dig:)', 5)
            record('DIG_AFTER_STOP ' + answer)
        except TimeoutError:
            record('DIG_AFTER_STOP no completion in five seconds')
        bot('block 12 161 13')
        rcon('execute if block 12 161 13 air run say DIG_CONTINUED_AFTER_STOP')
        wait_for(r'robot: (?:goto |error goto:)', 10)
        bot('state')
except Exception as error:
    record('RUN_ERROR ' + repr(error))
    steps.append({'ok': False, 'error': repr(error)})
finally:
    if proc.poll() is None:
        send('quit')
        try:
            proc.wait(timeout=8)
        except subprocess.TimeoutExpired:
            proc.terminate()
            proc.wait(timeout=5)
    thread.join(timeout=2)
    record(f'EXIT_CODE {proc.returncode}')
    (OUT / 'results.json').write_text(json.dumps({
        'server': 'Paper 26.2 build 134, offline, no plugins',
        'kit': 'f742fe252fe301624946de5b5a562ee000c8c9a0',
        'protocol_library': 'v0.262.2', 'race_enabled': True,
        'label': label, 'binary': binary,
        'exit_code': proc.returncode, 'steps': steps,
        'total_seconds': round(time.monotonic()-start, 3)}, indent=2) + '\n')
    transcript.close()
sys.exit(1 if proc.returncode or any(not step['ok'] for step in steps) else 0)
