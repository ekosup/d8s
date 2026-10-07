# d8s

[English](README.md)

TUI untuk Docker Engine dan Docker Swarm, dengan cara pakai seperti [k9s](https://k9scli.io): tabel yang selalu live, navigasi keyboard, perintah `:`, dan filter `/`.

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

d8s hanya berbicara ke Docker API. Tidak ada yang dipasang di server, dan binary `docker` tidak dibutuhkan.

## Pasang

Dari halaman [Releases](https://github.com/ekosup/d8s/releases), ambil berkas untuk platform Anda:

```bash
# Debian / Ubuntu
sudo dpkg -i d8s_<versi>_linux_amd64.deb

# Fedora / RHEL
sudo rpm -i d8s_<versi>_linux_amd64.rpm

# Linux atau macOS, tanpa paket
tar -xzf d8s_<versi>_linux_amd64.tar.gz && sudo mv d8s /usr/local/bin/
```

Atau dari sumber, dengan Go 1.24 ke atas:

```bash
go install github.com/ekosup/d8s/cmd/d8s@latest
```

Atau dari salinan repo ini, ke `~/.local/bin` (atau `PREFIX` lain):

```bash
make install
```

Periksa hasilnya:

```bash
d8s version
d8s info        # versi, context, engine, dan lokasi berkas yang dipakai
```

d8s memakai Docker context yang sedang aktif, sama seperti `docker`. Kalau `docker ps` jalan di terminal Anda, `d8s` juga.

## Lima menit pertama

1. Jalankan `d8s`. Daftar container tampil dan memperbarui diri sendiri. (Bila terhubung ke manager Swarm, yang tampil lebih dulu adalah daftar service; `:c` membuka container.)
2. Gerakkan sorotan dengan `j` / `k` atau panah.
3. Tekan `l` untuk melihat log container yang disorot. `Esc` untuk kembali.
4. Tekan `/`, ketik sebagian nama, `Enter` untuk menyaring. `Esc` menghapus saringan.
5. Tekan `:` lalu ketik `i` dan `Enter` untuk pindah ke daftar image.
6. Tekan `?` kapan saja untuk melihat semua tombol dan perintah.
7. `:q` atau `Ctrl-c` untuk keluar.

## Perintah

Ketik `:` diikuti salah satu dari ini. Nama lengkap dan awalan yang unik juga diterima (`:containers`, `:cont`).

| Perintah | View | Butuh Swarm |
| --- | --- | --- |
| `:c` | Container | |
| `:i` | Image | |
| `:v` | Volume | |
| `:n` | Network | |
| `:cp` | Project Compose | |
| `:df` | Pemakaian disk | |
| `:ctx` | Docker context | |
| `:ev` | Event daemon | |
| `:svc` | Service | ya |
| `:ts` | Task | ya |
| `:no` | Node | ya |
| `:stk` | Stack | ya |
| `:sec` | Secret | ya |
| `:cfg` | Config | ya |

## Tombol

Header selalu menampilkan tombol yang berlaku di view yang sedang dibuka, dan `?` menampilkan semuanya.

**Di mana saja**

| Tombol | Fungsi |
| --- | --- |
| `:` | Perintah |
| `/` | Saring tabel, atau cari di log dan inspect |
| `?` | Bantuan |
| `Enter` | Masuk ke isi baris (misalnya dari service ke task-nya) |
| `Esc` | Kembali; menghapus saringan dan tanda lebih dulu |
| `d`, `y` | Inspect sebagai JSON, atau YAML |
| `Shift` + huruf | Urutkan menurut kolom; tekan lagi untuk membalik |
| `Space` | Tandai baris untuk aksi massal |
| `Ctrl-d` | Hapus, dengan konfirmasi |
| `1`–`9` | Pindah ke Docker context itu, di tabel mana pun |

**Container**

| Tombol | Fungsi |
| --- | --- |
| `l` | Log |
| `s` | Shell (mencoba `bash`, lalu `sh`) |
| `m` | Statistik CPU, memori, jaringan, dan disk yang live |
| `a`, `x`, `r`, `p` | Start, stop, restart, pause atau resume |
| `Ctrl-k` | Kill |
| `h` | Sembunyikan atau tampilkan container yang tidak aktif |

**Log dan inspect**

| Tombol | Fungsi |
| --- | --- |
| `n`, `Shift-n` | Hasil cari berikut, sebelumnya |
| `w` | Bungkus baris panjang |
| `s` | Jeda atau lanjutkan autoscroll |
| `t` | Tampilkan timestamp |
| `0`–`5` | Rentang: 1.000 baris terakhir, 1, 5, 15, 30 menit, 1 jam |
| `c` | Salin ke clipboard |
| `Ctrl-s` | Simpan ke `~/.local/state/d8s/dumps/` |

**Swarm**

| View | Tombol | Fungsi |
| --- | --- | --- |
| Service | `s` | Scale |
| Service | `i` | Ganti image |
| Service | `r` | Restart semua task |
| Service | `u` | Rollback ke spesifikasi sebelumnya |
| Service | `o` | Pantau rollout: task baru dan lama, status, pesan jeda |
| Service, task | `l` | Log lewat manager, tiap baris berawalan `task@node` |
| Task | `h` | Sembunyikan atau tampilkan riwayat task |
| Task | `s` | Shell, bila container ada di node yang terhubung |
| Node | `a` | Availability: active, pause, drain |
| Node | `p` | Promote atau demote |
| Node | `b` | Pasang (`kunci=nilai`) atau hapus (`kunci-`) label |

Menghapus service atau stack meminta namanya diketik.

## Swarm

View Swarm butuh koneksi ke node **manager**. Dari satu manager, seluruh cluster terlihat dan bisa dioperasikan, termasuk log task di node lain.

Dua hal hanya menjangkau container di node yang sedang terhubung, karena begitulah Docker API bekerja: shell dan statistik. Untuk task di node lain, d8s menjelaskan sebabnya dan, bila ada Docker context yang namanya sama dengan hostname node itu, menawarkan pindah ke sana. Karena alasan yang sama, daftar container (`:c`) hanya memuat container di node yang terhubung; gambaran seluruh cluster ada di `:svc`, `:ts`, dan `:stk`.

Menghubungkan ke server lain memakai Docker context biasa:

```bash
docker context create prod --docker host=ssh://user@manager.example.com
d8s --context prod
```

Di dalam d8s ada tiga cara berpindah context tanpa keluar:

- `1` sampai `9` di tabel mana pun. Nomornya mengikuti urutan daftar context, tampil di header bila terminal cukup lebar, dan selalu ada di `?`.
- `:ctx prod`, atau awalan nama yang unik (`:ctx pr`); `Tab` melengkapinya.
- `:ctx`, lalu `Enter` pada sebuah baris. Daftar itu juga menunjukkan context mana yang read-only dan mana yang ditandai produksi.

## Mode read-only

```bash
d8s --readonly
```

Semua aksi yang mengubah keadaan ditolak, termasuk shell; melihat, mencari, dan membaca log tetap bisa. Header menampilkan `READ-ONLY`. Mode ini juga bisa dipasang permanen per context lewat konfigurasi.

## Konfigurasi

Opsional. Berkasnya `~/.config/d8s/config.yaml` (atau `$XDG_CONFIG_HOME/d8s/config.yaml`, atau path di `--config` / `$D8S_CONFIG`). Tanpa berkas, nilai bawaan di bawah ini yang berlaku.

```yaml
refresh: 2s              # seberapa sering view memeriksa ulang; minimal 500ms
defaultView: auto        # view pertama; auto = service di manager Swarm, selain itu container
logBuffer: 5000          # baris yang disimpan satu halaman log
logTail: 1000            # baris yang diambil saat log dibuka
shell: ""                # shell di container; kosong = bash, lalu sh
readOnly: false          # tolak semua perubahan, di semua context
skin: dark               # dark, light, atau mono

contexts:
  prod:
    readOnly: true       # context ini selalu read-only
    production: true     # ditandai di header

aliases:
  web: services /shop_web   # :web membuka service, tersaring

hotkeys:
  f2: svc                   # F2 menjalankan :svc
  ctrl-w: web

views:
  containers:
    columns: [NAME, STATE, CPU%, MEM, AGE]   # kolom yang tampil, berurutan
```

Kesalahan di berkas ini dilaporkan beserta nomor barisnya, dan d8s tidak mau jalan sampai diperbaiki. Alias atau hotkey yang bentrok dengan bawaan diabaikan dan dilaporkan saat mulai. Begitu juga nama di bawah `contexts` yang bukan Docker context: pengaturannya tidak berlaku untuk apa pun, dan `d8s info` ikut menampilkannya.

Alias atau hotkey boleh menyebut context: `prod: ctx prod` membuat `:prod` langsung pindah ke sana.

Urutan sort tiap view diingat sendiri antarsesi di `~/.local/state/d8s/state.yaml`.

**Tanpa warna.** Skin `mono`, atau variabel lingkungan `NO_COLOR`, mematikan semua warna; status tetap terbedakan lewat tebal, garis bawah, dan redup.

## Untuk pengguna k9s

Yang sama: `:` perintah, `/` saring, `?` bantuan, `Enter` masuk, `Esc` kembali, `d` describe, `y` YAML, `l` log, `s` shell, `Ctrl-d` hapus, `Ctrl-k` kill, `Space` tandai, `Shift` + huruf untuk sort, `:ctx` ganti context.

Yang berbeda:

| k9s | d8s | Catatan |
| --- | --- | --- |
| `:po`, `:deploy` | `:c`, `:svc` | Pod kira-kira container atau task; deployment kira-kira service |
| `:ns` | — | Docker tidak punya namespace; stack (`:stk`) yang paling dekat |
| `0`–`9` pindah namespace | `1`–`9` pindah context | Tempatnya sama di header |
| `e` edit | — | d8s tidak mengedit spesifikasi; ada `s` scale dan `i` ganti image |
| `s` di deployment = scale | `s` di service = scale | Di container, `s` tetap shell |
| `Ctrl-a` daftar alias | `?` | Daftar perintah ada di layar bantuan |

## Batasan

- Tidak membuat apa pun: deploy stack, membuat service, secret, atau config dilakukan dengan `docker`.
- Tidak membangun atau mendorong image.
- Statistik CPU dan memori hanya untuk container di daemon yang terhubung, dan dimatikan bila lebih dari 100 container berjalan.
- Shell interaktif butuh `/dev/tty`, jadi belum berfungsi di Windows.
- Diuji terhadap Docker Engine 20.10 (API 1.41) sampai 29; Podman tidak didukung.

## Pengembangan

```bash
make tools             # pasang linter dan alat rilis ke bin/tools
make build             # bin/d8s
make install           # salin ke ~/.local/bin
make test lint         # unit test dan lint
make demo-up           # container, network, dan volume d8s-demo* untuk dicoba
make swarm-up          # Swarm tiga node di dalam container, context d8s-swarm
make test-integration  # test terhadap cluster itu
make test-matrix       # hal yang sama terhadap engine tertua dan terbaru
make bench             # ukur terhadap target performa
make release-snapshot  # semua artefak rilis ke dist/, tanpa mempublikasikan
make release-status    # apa yang belum dirilis, dan rilis apa berikutnya
make release-next PART=patch   # test, naikkan versi, tag, push, dan terbitkan (token dari .env)
```

`make swarm-up` tidak mengubah daemon Anda menjadi anggota Swarm: clusternya hidup di tiga container. Spesifikasi produk ada di [docs/PRD.md](docs/PRD.md) dan rencana kerjanya di [docs/BACKLOG.md](docs/BACKLOG.md).

## Lisensi

[0BSD](LICENSE): boleh dipakai, disalin, diubah, dan disebarkan untuk tujuan apa pun, tanpa syarat apa pun, termasuk tanpa kewajiban mencantumkan nama pembuatnya.

Dipelihara oleh Eko Supriyono <esup0001@gmail.com>.
