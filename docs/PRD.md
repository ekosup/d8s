# d8s — PRD

Per 2026-10-06. Backlog ada di [BACKLOG.md](BACKLOG.md).

## Ringkasan

d8s adalah TUI satu binary untuk mengamati dan mengoperasikan Docker Engine dan Docker Swarm dengan pola interaksi k9s: tabel resource yang live, navigasi keyboard, command mode `:`, dan filter `/`.

Keputusan lingkup:

- **Swarm adalah warga kelas satu.** Service, task, node, stack, secret, dan config punya view dan aksi sendiri, bukan sekadar tambahan di atas daftar container.
- **Tanpa komponen di server.** d8s hanya berbicara ke Docker API lewat konteks yang sudah ada (socket lokal, TCP+TLS, atau SSH). Tidak ada yang dipasang di server.
- **Operasi, bukan authoring.** d8s mengamati dan mengubah resource yang sudah berjalan. Menulis Compose file, build image, dan deploy stack dari file berada di luar 1.0.
- **Empat milestone, 48 story, sekitar 120 poin.** M0 fondasi, M1 Docker Engine, M2 Swarm, M3 poles dan rilis 1.0. Tanggal belum ditetapkan karena kapasitas tim belum diketahui.

## Masalah dan latar belakang

Mendiagnosa satu service Swarm yang gagal lewat CLI butuh empat sampai lima perintah dan beberapa kali salin ID: `service ls`, `service ps --no-trunc`, `service logs`, lalu `inspect`. k9s sudah menghapus rasa sakit yang sama di Kubernetes; Docker dan terutama Swarm belum punya padanannya.

Tiga celah yang ingin ditutup:

- **Tidak ada tampilan live.** Setiap pertanyaan adalah satu perintah baru. Rollout yang sedang berjalan hanya bisa diikuti dengan `watch`.
- **Menelusuri relasi itu manual.** Stack ke service, service ke task, task ke container dan log: semuanya lewat ID yang disalin tangan.
- **Swarm kurang terlayani.** Tool terminal yang populer berfokus pada container di satu host.

| Tool | Bentuk | Swarm | Celah terhadap d8s |
| --- | --- | --- | --- |
| docker CLI | Perintah satu per satu | Lengkap | Tidak live, tidak ada drill-down |
| lazydocker | TUI panel | Tidak | Hanya container, image, volume, dan Compose di satu host |
| ctop | TUI metrik | Tidak | Hanya metrik container |
| dry | TUI | Ya | Pola interaksi sendiri, bukan model k9s |
| Portainer | Web UI | Ya | Harus dipasang dan dijalankan sebagai layanan |

Tabel ini disusun dari ingatan, belum dicek ulang ke halaman tiap proyek. Anggap sebagai perkiraan sampai diverifikasi.

## Pengguna dan skenario

Pengguna utama adalah engineer yang mengoperasikan cluster Swarm kecil sampai menengah dari terminal. Developer dengan Docker lokal adalah pengguna kedua, dan merekalah yang paling awal bisa memakai d8s (M1).

| Pengguna | Konteks kerja | Yang paling dibutuhkan |
| --- | --- | --- |
| DevOps / SRE pengelola Swarm | Beberapa cluster (staging, produksi), akses lewat Docker context atau SSH | Status rollout, task gagal beserta alasannya, log service, scale dan rollback |
| Engineer on-call | SSH ke manager node saat insiden, waktu sempit | Jalan tercepat dari gejala ke log, tanpa menghafal perintah |
| Backend developer | Docker lokal dan Compose di laptop | Log, shell, restart, dan bersih-bersih image dan volume |

Skenario yang harus mulus:

1. **Rollout gagal.** Buka service, lihat replika 2/5, masuk ke task, baca error lengkap task yang gagal, buka log-nya, lalu rollback.
2. **Container bermasalah di lokal.** Urutkan container menurut memori, buka log, masuk shell, restart.
3. **Pindah lingkungan.** Ganti konteks dari staging ke produksi tanpa keluar dari aplikasi, dengan penanda jelas sedang berada di mana.
4. **Maintenance node.** Drain node, pantau task berpindah, kembalikan ke active.
5. **Bersih-bersih.** Lihat pemakaian disk, hapus image dangling dan volume tak terpakai.

## Tujuan, non-tujuan, dan prinsip

d8s 1.0 berhasil bila skenario rollout gagal bisa diselesaikan dari satu layar, tanpa mengetik satu ID pun.

### Tujuan

- Menggantikan pemakaian harian `docker ps`, `logs`, `exec`, `stats`, `service ls`, `service ps`, `service logs`, dan `node ls`.
- Dari membuka aplikasi sampai membaca log task yang gagal: paling banyak 10 penekanan tombol.
- Pengguna k9s produktif tanpa membaca dokumentasi, karena tombol dan alurnya sama.
- Tabel pertama tampil di bawah 300 ms pada daemon lokal.

### Non-tujuan untuk 1.0

- Menulis atau mengedit Compose file, dan deploy stack dari file.
- Build dan push image, serta pengelolaan registry.
- Kubernetes, dan dukungan resmi untuk Podman.
- Web UI, atau daemon pendamping di node.
- Penyimpanan metrik historis dan alerting. d8s hanya menampilkan keadaan saat ini.

### Prinsip produk

- **Keyboard dulu, dan akrab bagi pengguna k9s.** Bila k9s punya tombol untuk suatu hal, d8s memakai tombol yang sama.
- **Aman secara default.** Aksi destruktif selalu minta konfirmasi, konteks aktif selalu terlihat, dan ada mode read-only.
- **Nol jejak di server.** Yang dibutuhkan hanya akses ke Docker API.
- **Jujur soal batas Swarm.** Bila sesuatu tidak bisa dilakukan lewat manager, d8s mengatakannya dan menawarkan jalan lain, bukan gagal diam-diam.

## Kebutuhan fungsional

d8s 1.0 mencakup 14 view resource: lima untuk Docker Engine, enam untuk Swarm, dan tiga view sistem. Semuanya memakai satu komponen tabel yang sama.

| Resource | Perintah | Kolom utama | Aksi | Milestone |
| --- | --- | --- | --- | --- |
| Container | `:c` | Nama, image, state, status, port, CPU, memori, umur | Log, shell, inspect, start, stop, restart, pause, kill, hapus | M0–M1 |
| Image | `:i` | Repo, tag, ID, ukuran, umur, dipakai | Inspect, riwayat layer, hapus, prune dangling | M1 |
| Volume | `:v` | Nama, driver, dipakai oleh, umur | Inspect, hapus, prune | M1 |
| Network | `:n` | Nama, driver, scope, subnet, jumlah container | Inspect, hapus | M1 |
| Compose project | `:compose` | Project, jumlah container, berjalan, direktori kerja | Start, stop, restart seluruh project | M1 |
| Service | `:svc` | Nama, stack, mode, replika berjalan/diinginkan, image, port, status update | Log, inspect, scale, force update, ganti image, rollback, hapus | M2 |
| Task | `:tasks` | Service, slot, node, state diinginkan, state kini, error lengkap, umur | Log, inspect, shell bila terjangkau | M2 |
| Node | `:no` | Hostname, status, availability, peran, leader, versi engine | Inspect, drain, active, pause, promote, demote, label | M2 |
| Stack | `:stk` | Nama, jumlah service, replika sehat | Hapus stack | M2 |
| Secret | `:sec` | Nama, dibuat, diperbarui, dipakai oleh | Inspect metadata, hapus | M2 |
| Config | `:cfg` | Nama, dibuat, dipakai oleh | Lihat isi, hapus | M2 |
| Context | `:ctx` | Nama, endpoint, aktif | Pindah konteks | M1 |
| Events | `:ev` | Waktu, tipe, aksi, aktor | Filter | M1 |
| Disk usage | `:df` | Jenis, jumlah, ukuran, bisa dibebaskan | Prune terpandu | M1 |

Compose project dan stack bukan objek di Docker API. Keduanya diturunkan dari label `com.docker.compose.project` dan `com.docker.stack.namespace`.

### Kemampuan lintas resource

- **Log.** Follow, pilihan rentang waktu dan jumlah baris awal, timestamp, wrap, pencarian, dan simpan ke file. Log service menggabungkan semua task dengan awalan nama task dan node.
- **Shell.** Exec interaktif ke container dengan TTY penuh; TUI ditangguhkan lalu dipulihkan saat keluar. Mencoba `bash`, jatuh ke `sh`.
- **Inspect.** Tampilan YAML atau JSON dari objek mentah, dengan pencarian dan salin.
- **Stats.** CPU, memori, jaringan, dan block IO live per container, sebagai kolom tabel dan view detail.
- **Drill-down.** `Enter` selalu turun ke anak resource; `Esc` kembali. Lihat peta navigasi di bagian UX.
- **Filter dan sort.** Filter `/` dengan substring, regex, atau kebalikan (`/!teks`). Sort per kolom.
- **Multi-seleksi.** `Space` menandai baris; aksi berlaku untuk semua yang ditandai.
- **Mode read-only.** Semua aksi yang mengubah keadaan dinonaktifkan, per sesi atau per konteks.

### Batas Swarm yang memengaruhi desain

- Log service dan task bisa diambil lewat manager, jadi selalu tersedia.
- Exec dan stats hanya bekerja pada daemon tempat container itu berjalan. Untuk task di node lain, d8s menampilkan alasannya dan menawarkan pindah ke konteks node tersebut bila ada.
- Nilai secret tidak pernah dikembalikan API. d8s hanya menampilkan metadatanya.
- View Swarm hanya berfungsi bila terhubung ke manager. Di worker atau engine non-Swarm, view itu menampilkan pesan yang menjelaskan sebabnya.

## UX

Layar d8s meniru k9s: header berisi konteks dan petunjuk tombol, satu tabel besar di tengah, breadcrumb dan pesan status di bawah. Tidak ada mouse yang diwajibkan dan tidak ada panel bertumpuk.

```text
 Context: prod-swarm   Engine: 27.3.1   Swarm: manager (leader)   <:> cmd  </> filter  <?> help
 Nodes: 5/5 ready      Services: 18     Mode: READ-ONLY           <l> logs <s> shell <d> inspect
┌─ Services(all)[18] ──────────────────────────────────────────────────────────────────────────┐
│ NAME            STACK   MODE        REPLICAS  IMAGE                 UPDATE      AGE          │
│ shop_api        shop    replicated  2/5       registry/api:1.8.2    updating    3m           │
│ shop_web        shop    replicated  3/3       registry/web:4.1.0    completed   2d           │
│ mon_exporter    mon     global      5/5       prom/node-exporter    -           41d          │
└──────────────────────────────────────────────────────────────────────────────────────────────┘
 <stacks> <services>                                              shop_api: 3 task gagal (non-zero exit 137)
```

Baris dengan replika kurang dari yang diinginkan diberi warna peringatan; task gagal berwarna kritis. Nama konteks yang ditandai produksi di konfigurasi tampil dengan warna berbeda di header.

### Tombol

| Tombol | Fungsi | Sama dengan k9s |
| --- | --- | --- |
| `:` | Command mode: pindah view lewat nama atau alias, dengan autocomplete | Ya |
| `/` | Filter tabel; `Esc` menghapus filter | Ya |
| `Enter` | Drill-down ke anak resource | Ya |
| `Esc` | Kembali satu tingkat | Ya |
| `?` | Bantuan dan daftar tombol untuk view aktif | Ya |
| `d` | Inspect | Ya (describe) |
| `y` | Objek mentah sebagai YAML | Ya |
| `l` | Log | Ya |
| `s` | Shell | Ya |
| `Ctrl-d` | Hapus, dengan konfirmasi | Ya |
| `Ctrl-k` | Kill, dengan konfirmasi | Ya |
| `Space` | Tandai baris | Ya |
| `Shift` + huruf | Sort menurut kolom | Ya |
| `n`, `Shift-n` | Kecocokan berikut dan sebelumnya di inspect dan log | Ya |
| `a`, `x`, `r` | Start, stop, restart container; `r` juga force update service | Baru |
| `p` | Pause atau resume container | Baru |
| `m` | Halaman stats live | Baru |
| `h` | Riwayat layer image | Baru |
| `Ctrl-p` | Prune, dengan konfirmasi | Baru |
| `S` | Scale service | Baru |
| `u` | Rollback service ke spesifikasi sebelumnya | Baru |
| `:q`, `Ctrl-c` | Keluar | Ya |

### Peta drill-down

- Stack → service → task → log atau inspect
- Node → task di node itu → log atau inspect
- Compose project → container → log, shell, atau inspect
- Image → container yang memakainya
- Volume → container yang memasangnya
- Network → container yang terhubung

### Konfirmasi

Hapus dan kill meminta konfirmasi ya/tidak. Menghapus service atau stack meminta pengguna mengetik namanya.

## Arsitektur teknis

d8s ditulis dalam Go sebagai satu binary statis, dengan tiga lapis: TUI, inti yang tidak tahu soal terminal, dan akses Docker di balik satu antarmuka.

```text
┌─ TUI: tview + tcell ─────────────────────────────────────────────────────────┐
│  Shell dan router          View                      Dialog dan prompt       │
│  tumpukan halaman,         tabel, log, inspect,      konfirmasi aksi         │
│  breadcrumb, command       stats; satu view          destruktif, input       │
│  mode, keymap              generik per jenis         scale dan filter        │
└──────────────────────────────────────────────────────────────────────────────┘
                 ▲▼  perintah turun, snapshot tabel naik
┌─ Inti: tidak tahu soal terminal ─────────────────────────────────────────────┐
│  Registry resource         Store dan watcher         Eksekutor aksi          │
│  deklarasi kolom, alias,   cache snapshot; refresh   menjalankan aksi,       │
│  aksi, dan drill-down      dari event dan polling    menghormati read-only,  │
│  per resource              task Swarm                melaporkan hasil        │
└──────────────────────────────────────────────────────────────────────────────┘
                 ▲▼  list, inspect, dan aksi turun; event naik
┌─ Akses Docker: di balik satu antarmuka ──────────────────────────────────────┐
│  Pemuat konteks            Klien Docker API          Sesi stream             │
│  Docker context,           SDK Go resmi, negosiasi   log, stats, events,     │
│  DOCKER_HOST, TLS,         versi API, deteksi        dan exec dengan TTY     │
│  dan ssh://                mode Swarm                interaktif              │
└──────────────────────────────────────────────────────────────────────────────┘
                 ▲▼  HTTP lewat unix socket, TCP+TLS, atau SSH
                   Docker Engine atau Swarm manager
```

Perintah pengguna hanya mengalir turun dan data hanya mengalir naik. Lapis inti bisa diuji dengan klien Docker palsu, tanpa terminal dan tanpa daemon.

| Keputusan | Pilihan | Alasan |
| --- | --- | --- |
| Bahasa | Go | Satu binary statis, SDK Docker resmi ada di Go, sama dengan k9s |
| Library TUI | tview di atas tcell | Dipakai k9s; kuat untuk tabel besar dan tumpukan halaman. Alternatifnya Bubble Tea, lihat pertanyaan terbuka |
| Klien Docker | SDK Go resmi dengan negosiasi versi API | Tidak bergantung pada binary `docker` di mesin pengguna |
| Konteks | Membaca Docker context dan `DOCKER_HOST` yang sudah ada | Pengguna tidak mengonfigurasi koneksi dua kali |
| Refresh | Event stream untuk container, image, volume, network, service, node; polling untuk task | Task Swarm tidak memancarkan event |
| Definisi resource | Deklaratif di registry: kolom, alias, aksi, anak drill-down | Menambah resource baru tidak menyentuh kode view |
| Konfigurasi | YAML di `~/.config/d8s/` mengikuti XDG | Pola yang sudah dikenal dari k9s |

**Model refresh.** Hanya view yang sedang tampil yang di-refresh. Event memicu pengambilan ulang yang di-debounce; polling berjalan tiap 2 detik secara default dan bisa diatur. Stream log, stats, dan exec hidup selama view-nya terbuka dan ditutup saat pengguna keluar dari view itu.

### Struktur paket yang diusulkan

```text
cmd/d8s/            titik masuk, flag CLI
internal/ui/        shell, view tabel, log, inspect, dialog, keymap
internal/resource/  registry dan definisi tiap resource
internal/store/     cache snapshot, watcher, debounce
internal/action/    eksekutor aksi dan penjaga read-only
internal/docker/    antarmuka klien, pemuat konteks, sesi stream
internal/config/    konfigurasi, skin, alias, hotkey
```

## Kebutuhan non-fungsional

Semua angka di bawah adalah target usulan untuk 1.0 dan diuji di M3 (D8S-044, D8S-046).

| Area | Kebutuhan |
| --- | --- |
| Performa | Tabel pertama di bawah 300 ms pada daemon lokal. Tabel 2.000 baris tetap responsif terhadap tombol. |
| Sumber daya | Memori di bawah 100 MB dengan 1.000 container. CPU di bawah 2% saat diam. Buffer log dibatasi, default 5.000 baris. |
| Kompatibilitas engine | Docker Engine API 1.41 ke atas (Engine 20.10), dengan negosiasi versi. Mesin pengembangan saat ini: Engine 27.3.1, API 1.47. |
| Platform | Linux dan macOS (amd64, arm64), Windows (amd64): lima target build. |
| Terminal | Minimal 80×24. Truecolor dan 256 warna, serta `NO_COLOR`. |
| Keamanan | Tidak menyimpan kredensial; memakai konfigurasi Docker yang ada. Tanpa telemetri. Nilai secret tidak pernah ditampilkan. |
| Keselamatan operasi | Konfirmasi untuk semua aksi destruktif. Mode read-only per sesi (`--readonly`) dan per konteks. |
| Keandalan | Koneksi putus ditandai di header dan disambung ulang otomatis. Terminal selalu dipulihkan, termasuk saat panic. |
| Distribusi | GitHub Releases dengan checksum, Homebrew, paket deb dan rpm, dan `go install`. |
| Kualitas | Unit test lapis inti dengan klien palsu. Integration test terhadap engine dan Swarm multi-node sungguhan di CI. |

## Rencana rilis dan metrik sukses

d8s mencapai 1.0 lewat empat milestone berurutan dengan total 48 story dan 120 poin. Setiap milestone menghasilkan versi yang bisa dipakai, dan fase berikut tidak dimulai sebelum gate-nya lolos.

| Milestone | Versi | Isi | Ukuran | Gate keluar |
| --- | --- | --- | --- | --- |
| M0 · Fondasi | v0.1 | Kerangka TUI, klien Docker, tabel generik, command mode, filter | 10 story · 30 poin | Tabel container tampil live |
| M1 · Docker Engine | v0.3 | Container, image, volume, network, log, shell, stats, konteks | 14 story · 34 poin | Kerja harian engine tanpa CLI docker |
| M2 · Swarm | v0.6 | Service, task, node, stack, secret, config, scale dan rollback | 13 story · 33 poin | Rollout gagal bisa didiagnosa dari satu layar |
| M3 · Rilis 1.0 | v1.0 | Konfigurasi, skin, read-only, paket rilis, dokumentasi | 11 story · 23 poin | Lulus uji matriks, paket terpasang bersih |

Setiap milestone di-build dan dicek di VM dev (Linux x86_64, Docker Engine 27.3.1) sebelum milestone berikutnya dimulai; daftar ceknya ada di [BACKLOG.md](BACKLOG.md). macOS dan Windows hanya dibuktikan lewat build lintas platform di M3, tidak dijalankan.

M1 sudah berguna bagi developer lokal, jadi umpan balik bisa dikumpulkan sebelum pekerjaan Swarm dimulai.

Satu poin kira-kira satu hari kerja satu orang, jadi 120 poin setara sekitar 24 minggu untuk satu pengembang penuh waktu. Ini perkiraan kasar dari ukuran story, bukan komitmen tanggal.

### Metrik sukses 1.0

| Metrik | Target | Cara mengukur |
| --- | --- | --- |
| Skenario rollout gagal | Selesai tanpa mengetik ID dan tanpa keluar dari d8s | Uji skenario manual di cluster uji tiap rilis |
| Tombol sampai log task gagal | Paling banyak 10 | Dihitung pada skenario yang sama |
| Waktu ke tabel pertama | Di bawah 300 ms, daemon lokal | Benchmark di CI |
| Memori pada 1.000 container | Di bawah 100 MB | Uji beban sintetis (D8S-046) |
| Cakupan engine | Lulus di versi minimum dan versi terbaru | Matriks integration test (D8S-044) |
| Stabilitas | Tidak ada crash yang meninggalkan terminal rusak | Uji panic dan putus koneksi |

Target adopsi (unduhan, bintang, kontributor) belum ditetapkan.

## Risiko dan pertanyaan terbuka

Risiko terbesar adalah harapan pengguna k9s yang tidak bisa dipenuhi Swarm: shell dan stats untuk task di node lain tidak tersedia lewat manager.

| Risiko | Dampak | Mitigasi |
| --- | --- | --- |
| Exec dan stats tidak bisa menjangkau task di node lain | Alur "shell ke task" terasa rusak dibanding k9s | Pesan yang menjelaskan sebabnya, dan lompat ke konteks node itu bila ada (D8S-035) |
| Task Swarm tidak memancarkan event, jadi harus polling | Beban pada manager di cluster besar | Polling hanya untuk view aktif, interval bisa diatur, diuji di D8S-046 |
| Aksi destruktif di konteks yang salah | Service produksi terhapus | Warna konteks produksi, konfirmasi dengan mengetik nama, read-only per konteks (D8S-039) |
| SDK Docker berubah antarversi | Build rusak saat upgrade dependensi | Seluruh akses SDK di balik satu antarmuka internal |
| Pilihan library TUI ternyata membatasi | Tulis ulang lapis UI | Lapis inti tidak bergantung pada terminal, jadi hanya `internal/ui` yang terdampak |
| Perilaku terminal Windows untuk exec dan resize | Shell interaktif tidak stabil di Windows | Windows diuji di matriks rilis; bila perlu shell di Windows ditandai eksperimental di 1.0 |

### Pertanyaan terbuka

- [ ] Library TUI: tview (usulan dokumen ini) atau Bubble Tea?
- [ ] Lisensi: Apache-2.0 seperti k9s, atau MIT?
- [ ] Siapa yang mengerjakan dan berapa kapasitasnya? Tanggal milestone menunggu jawaban ini.
- [ ] Apakah deploy stack dari Compose file masuk 1.0, atau tetap pasca-1.0?
- [ ] Podman: tidak didukung, atau best-effort lewat API kompatibelnya?
- [ ] Apakah nama `d8s` masih bebas di GitHub dan Homebrew? Belum dicek.
- [ ] Verifikasi tabel perbandingan tool di bagian latar belakang.
