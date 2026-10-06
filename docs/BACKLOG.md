# d8s — Backlog

Per 2026-10-06. Produk dan lingkupnya dijelaskan di [PRD.md](PRD.md).

Backlog berisi 48 story dalam tujuh epic, dibagi ke empat milestone. Setiap milestone dieksekusi utuh, lalu **di-build dan dicek di VM dev** sebelum milestone berikutnya dimulai.

## Aturan eksekusi

1. Satu milestone dikerjakan sampai selesai, tidak dicampur dengan milestone lain.
2. Di akhir milestone, `make build` harus menghasilkan `bin/d8s` tanpa error dan `make test` harus hijau.
3. `bin/d8s version` mencetak versi milestone itu (`v0.1.0`, `v0.3.0`, `v0.6.0`, `v1.0.0`), dan commit-nya diberi tag yang sama.
4. Pekerjaan berhenti di situ. Daftar **Cek di VM dev** milestone itu dijalankan oleh pemilik produk di VM ini.
5. Milestone berikutnya baru dimulai setelah cek itu disetujui. Temuan dari cek diperbaiki dulu di milestone yang sama.

## VM dev

Semua build dan cek dirancang untuk mesin ini, sesuai keadaannya pada 2026-10-06:

| Hal | Keadaan |
| --- | --- |
| Platform | Linux x86_64, 16 CPU, 31 GB RAM |
| Go | 1.24.13 |
| Docker | Engine 27.3.1 (API 1.47), Compose v2.29.7; user `puspensi` anggota grup `docker` |
| Docker context | `default` (aktif) dan `rootless` |
| Swarm | Tidak aktif di daemon utama |
| Sudah ada | `make`, `git`, `tmux` |
| Belum ada | `golangci-lint`, `goreleaser`; dipasang lewat `make tools` di M0 dan M3 |

Dua hal yang dijaga selama cek:

- **Container yang sudah ada di VM tidak disentuh.** Daemon ini sudah memuat 11 container dan 74 image. Semua cek memakai resource berawalan `d8s-demo`, yang dibuat `make demo-up` dan dibersihkan `make demo-down`.
- **Swarm tidak diaktifkan di daemon utama.** Cluster uji M2 berjalan sebagai tiga container docker-in-docker (`make swarm-up`) dengan context sendiri bernama `d8s-swarm`.

## Cara membaca

- **Prioritas**: P0 wajib untuk gate milestone, P1 penting tetapi bisa bergeser satu milestone, P2 boleh dilepas dari 1.0.
- **Ukuran**: S = 1 poin (sampai satu hari), M = 3 poin (dua sampai tiga hari), L = 5 poin (sekitar satu minggu).
- Story di tiap milestone ditulis menurut urutan pengerjaan.

Jalur kritisnya adalah semua story P0: 27 story, 75 poin.

## Ringkasan

| Milestone | Versi | Story | Poin | Gate: lolos cek di VM dev bila… |
| --- | --- | --- | --- | --- |
| [M0 · Fondasi](#m0--fondasi-v01) | v0.1.0 | 10 | 30 | Tabel container tampil live |
| [M1 · Docker Engine](#m1--docker-engine-v03) | v0.3.0 | 14 | 34 | Kerja harian engine selesai tanpa CLI docker |
| [M2 · Swarm](#m2--swarm-v06) | v0.6.0 | 13 | 33 | Rollout gagal terdiagnosa dari satu layar di cluster uji |
| [M3 · Rilis 1.0](#m3--rilis-10-v10) | v1.0.0 | 11 | 23 | Paket rilis terpasang dan lulus matriks engine |

| Epic | Isi | Story | Poin |
| --- | --- | --- | --- |
| Fondasi | Kerangka aplikasi, tabel generik, registry, store | 5 | 17 |
| Koneksi | Klien Docker, konteks, remote, reconnect | 4 | 12 |
| Engine | Container, image, volume, network, Compose, disk | 7 | 15 |
| Observasi | Log, shell, inspect, stats, events | 5 | 13 |
| Swarm | Service, task, node, stack, secret, config, rollout | 12 | 30 |
| UX | Command mode, filter, bantuan, seleksi, konfigurasi, skin | 9 | 19 |
| Kualitas | Lingkungan uji, rilis, dokumentasi, performa | 6 | 14 |

---

## M0 · Fondasi (v0.1)

**Tujuan.** Kerangka aplikasi berdiri dan satu view (container) tampil live. 10 story, 30 poin.

### Story

- [x] **D8S-001 — Inisialisasi repo Go: struktur paket, Makefile, lint** `Fondasi · P0 · S`
  - Terima: `make build`, `make test`, dan `make lint` berjalan hijau di VM dev. `make tools` memasang `golangci-lint`. `make demo-up` dan `make demo-down` membuat dan menghapus container `d8s-demo-*`.
- [x] **D8S-002 — Klien Docker: konek dari context aktif atau `DOCKER_HOST`, negosiasi versi API, ping** `Koneksi · P0 · M`
  - Terima: konek ke socket lokal; daemon mati atau izin ditolak menghasilkan pesan yang menyebut sebabnya.
- [x] **D8S-003 — Kerangka TUI: header, area utama, baris status, tumpukan halaman** `Fondasi · P0 · M`
  - Terima: halaman bisa ditumpuk dan dilepas dengan `Esc`; resize terminal tidak merusak tata letak.
- [x] **D8S-004 — Komponen tabel generik: kolom deklaratif, seleksi, scroll, sort** `Fondasi · P0 · L`
  - Terima: satu komponen dipakai semua view; 2.000 baris tetap responsif (dibuktikan dengan unit test dan benchmark).
- [x] **D8S-005 — Registry resource: antarmuka list, kolom, aksi, dan anak drill-down** `Fondasi · P0 · M`
  - Terima: resource baru didaftarkan tanpa mengubah kode view.
- [x] **D8S-006 — View container: daftar live** `Engine · P0 · M`
  - Terima: perubahan state container tampil dalam 2 detik tanpa input pengguna.
- [x] **D8S-007 — Store dan watcher: refresh dari event dengan debounce, polling sebagai cadangan** `Fondasi · P0 · L`
  - Terima: hanya view aktif yang di-refresh; tidak ada goroutine bocor setelah pindah view.
- [x] **D8S-008 — Command mode `:` dengan alias dan autocomplete** `UX · P0 · M`
  - Terima: `:c`, `:containers`, dan awalan unik membuka view yang sama; perintah tak dikenal memberi pesan.
- [x] **D8S-009 — Filter `/`: substring, regex, dan kebalikan** `UX · P0 · M`
  - Terima: filter bertahan saat tabel refresh; `Esc` menghapusnya.
- [x] **D8S-010 — Bantuan `?` yang dibangkitkan dari keymap view aktif** `UX · P1 · S`
  - Terima: tombol yang tampil di bantuan selalu sama dengan yang benar-benar terikat.

### Build

```bash
make tools          # sekali saja: memasang golangci-lint
make build          # menghasilkan bin/d8s
make test lint      # harus hijau
./bin/d8s version   # v0.1.0 beserta commit
```

### Cek di VM dev

1. `./bin/d8s` membuka layar penuh. Header menampilkan context `default` dan Engine 27.3.1; tabel memuat container yang ada di VM.
2. Di terminal lain jalankan `make demo-up`. Baris `d8s-demo-*` muncul sendiri dalam 2 detik.
3. Ketik `/d8s-demo`. Hanya container demo yang tersisa. `Esc` mengembalikan semuanya.
4. Ketik `/!d8s-demo`. Container demo hilang dari tabel, sisanya tetap.
5. `docker stop d8s-demo-web` dari terminal lain. Kolom state baris itu berubah sendiri.
6. Sort dengan `Shift` + huruf pada dua kolom berbeda; urutan berubah.
7. Ketik `:cont` lalu `Tab`; terlengkapi menjadi `containers`. Ketik `:xyz`; baris status menampilkan pesan perintah tidak dikenal.
8. Tekan `?`; daftar tombol tampil. `Esc` menutupnya.
9. Ubah ukuran jendela terminal; tata letak menyesuaikan tanpa rusak.
10. `:q` keluar; terminal kembali normal. `make demo-down` membersihkan container demo.
11. `DOCKER_HOST=unix:///tidak/ada ./bin/d8s` berhenti dengan pesan yang menyebut socket tidak ditemukan.

**Belum ada di milestone ini:** aksi apa pun pada container (log, shell, restart, hapus). M0 hanya menampilkan.

---

## M1 · Docker Engine (v0.3)

**Tujuan.** d8s menggantikan CLI `docker` untuk pekerjaan harian di satu host. 14 story, 34 poin.

### Story

- [x] **D8S-011 — Aksi container: start, stop, restart, pause, kill, hapus** `Engine · P0 · M`
  - Terima: aksi destruktif meminta konfirmasi; hasil atau error tampil di baris status.
- [x] **D8S-014 — Inspect: tampilan YAML dan JSON dengan pencarian dan salin** `Observasi · P0 · S`
  - Terima: tersedia di setiap view resource lewat `d` dan `y`.
- [x] **D8S-012 — Log viewer: follow, rentang waktu, timestamp, wrap, cari, simpan ke file** `Observasi · P0 · L`
  - Terima: mengikuti log 1.000 baris per detik tanpa membekukan UI; buffer dibatasi sesuai konfigurasi.
- [x] **D8S-013 — Shell ke container dengan TTY penuh** `Observasi · P0 · M`
  - Terima: TUI ditangguhkan dan pulih utuh saat keluar; resize diteruskan; jatuh ke `sh` bila `bash` tidak ada.
- [x] **D8S-016 — View image: daftar, riwayat layer, hapus, prune dangling, drill-down ke container pemakai** `Engine · P0 · M`
  - Terima: image yang sedang dipakai ditandai; menghapusnya memberi peringatan yang jelas.
- [x] **D8S-020 — View context dan pindah konteks tanpa restart** `Koneksi · P0 · M`
  - Terima: semua watcher dan stream konteks lama ditutup; header menampilkan konteks baru.
- [x] **D8S-015 — Stats live: CPU, memori, jaringan, block IO sebagai kolom dan view detail** `Observasi · P1 · M`
  - Terima: kolom bisa diurutkan; stream stats berhenti saat view ditutup.
- [x] **D8S-017 — View volume: daftar, pemakai, hapus, prune** `Engine · P1 · S`
  - Terima: volume yang masih terpasang tidak bisa dihapus tanpa peringatan.
- [x] **D8S-018 — View network: daftar, container terhubung, hapus** `Engine · P1 · S`
  - Terima: `Enter` menampilkan container yang terhubung beserta alamat IP-nya.
- [x] **D8S-019 — View Compose project dari label, dengan start, stop, restart seluruh project** `Engine · P1 · M`
  - Terima: project terdeteksi tanpa membaca Compose file; `Enter` menampilkan container-nya.
- [x] **D8S-021 — Koneksi remote: TCP+TLS dan `ssh://`** `Koneksi · P1 · M`
  - Terima: konteks SSH yang berfungsi di CLI `docker` juga berfungsi di d8s.
- [x] **D8S-022 — Multi-seleksi dengan `Space` dan aksi massal** `UX · P1 · M`
  - Terima: konfirmasi menyebut jumlah dan nama resource yang terdampak.
- [x] **D8S-023 — View events live** `Observasi · P2 · S`
  - Terima: bisa difilter menurut tipe dan aksi.
- [x] **D8S-024 — View disk usage dan prune terpandu** `Engine · P2 · S`
  - Terima: menampilkan ruang yang bisa dibebaskan sebelum pengguna mengonfirmasi.

### Build

```bash
make build
make test lint
./bin/d8s version   # v0.3.0
make demo-up        # project Compose d8s-demo: container web, idle, dan spam (log deras), network d8s-demo-net, volume d8s-demo-data
```

### Cek di VM dev

Semua aksi di bawah dilakukan hanya pada resource `d8s-demo*`. Ketik `/d8s-demo` dulu supaya hanya itu yang tampil.

1. Pilih `d8s-demo-web`, tekan `r`. State berubah ke restarting lalu running, dan baris status melaporkan `Restart d8s-demo-web: done`. `x` menghentikan, `a` menyalakan lagi, `p` menjeda dan melanjutkan.
2. Tekan `d` padanya; hasil inspect tampil sebagai JSON. Ketik `/Mounts` lalu `Enter`; tampilan melompat ke kecocokan dan judul menunjukkan `[1/n]`; `n` dan `Shift-n` berpindah antar kecocokan. `Esc` dua kali kembali ke tabel. `y` menampilkan objek yang sama sebagai YAML.
3. Tekan `l` pada `d8s-demo-spam`. Log mengalir deras dan tombol tetap responsif. `t` menampilkan timestamp, `s` menjeda dan melanjutkan autoscroll, `/` mencari, `1` sampai `5` mengganti rentang waktu, `Ctrl-s` menyimpan ke `~/.local/state/d8s/dumps/`; buka filenya.
4. Tekan `s` pada `d8s-demo-web`; prompt shell muncul. Jalankan `ls`, ubah ukuran jendela lalu `stty size` (angkanya mengikuti), `exit`. Tabel d8s kembali utuh dan tombol langsung berfungsi. Image demo tidak punya `bash`, jadi langkah ini sekaligus membuktikan jatuh ke `sh`.
5. Kolom CPU% dan MEM berubah tiap refresh. `Shift-c` dua kali mengurutkan CPU menurun; `d8s-demo-spam` naik ke atas dengan sekitar 100%. `m` membuka halaman stats yang diperbarui tiap detik.
6. `:i` menampilkan image; `/nginx` menyaring. `Enter` pada `nginx:alpine` menampilkan container pemakainya; `Esc` kembali. `h` menampilkan riwayat layer. `Ctrl-d` pada image yang dipakai memunculkan peringatan jumlah pemakainya; jawab `n`.
7. `:v` lalu `/d8s` menampilkan `d8s-demo-data` beserta pemakainya. `:n` lalu `/d8s` menampilkan `d8s-demo-net`; `Enter` menampilkan ketiga container beserta IP-nya.
8. `:compose` menampilkan project `d8s-demo` dengan `3/3`. `Enter` menampilkan container-nya; `Esc`, lalu `r` me-restart semuanya.
9. Kembali ke `:c`, `/d8s-demo`. Tandai `d8s-demo-idle` dan `d8s-demo-spam` dengan `Space`, tekan `Ctrl-d`. Dialog menyebut `Delete 2 items` dan kedua namanya; setelah `y` keduanya hilang dan `d8s-demo-web` tetap ada.
10. `:ctx` menampilkan `default` (bertanda `*`) dan `rootless`. `Enter` pada `rootless`: bila daemon rootless mati, muncul pesan socket tidak ditemukan dan koneksi tetap di `default`; bila hidup, header berganti dan tabel memuat isi daemon itu.
11. `:ev` terbuka; `docker restart d8s-demo-web` dari terminal lain memunculkan event-nya di baris teratas.
12. `:df` menampilkan pemakaian disk per jenis. `Ctrl-p` pada sebuah baris menampilkan apa yang akan dihapus dan berapa yang bisa dibebaskan; **jawab `n`**, karena prune berlaku untuk seluruh daemon, bukan hanya resource demo.
13. `?` menampilkan semua tombol view aktif dalam kolom-kolom.
14. `make demo-down`; semua resource demo hilang dari tabel dengan sendirinya.

**Tidak bisa dicek di VM ini:** koneksi `ssh://` (D8S-021). sshd aktif, tetapi kunci SSH user ini tidak terdaftar di `authorized_keys` localhost, dan itu sengaja tidak diubah. Story ini terbukti lewat unit test (penyusunan perintah `ssh` dan transport lewat proses anak); cek sungguhan butuh host yang bisa di-SSH dengan kunci. Koneksi TCP+TLS juga hanya terbukti lewat test pembacaan sertifikat.

**Catatan:** view container hasil drill-down (dari image, volume, network, atau project) belum memuat kolom CPU% dan MEM; kolom itu hanya ada di `:c`.

---

## M2 · Swarm (v0.6)

**Tujuan.** Swarm menjadi warga kelas satu: semua resource Swarm punya view dan aksinya. 13 story, 33 poin.

### Story

- [x] **D8S-037 — Lingkungan uji Swarm multi-node untuk integration test** `Kualitas · P0 · M`
  - Terima: `make swarm-up` menaikkan satu manager dan dua worker sebagai container docker-in-docker di VM dev, membuat context `d8s-swarm`, dan men-deploy stack contoh. `make swarm-down` menghapus semuanya. Daemon utama VM tidak masuk Swarm.
- [x] **D8S-025 — Deteksi mode Swarm dan peran node** `Swarm · P0 · S`
  - Terima: di worker atau engine non-Swarm, view Swarm menampilkan pesan penjelas, bukan error mentah.
- [x] **D8S-028 — View node: status, availability, peran, versi engine** `Swarm · P0 · S`
  - Terima: node down dan node drain dibedakan secara visual; `Enter` menampilkan task di node itu.
- [x] **D8S-026 — View service: mode, replika berjalan/diinginkan, image, port, status update** `Swarm · P0 · M`
  - Terima: service dengan replika kurang diberi warna peringatan; service global menampilkan jumlah node.
- [x] **D8S-027 — View task: state, error lengkap, node, riwayat; drill-down dari service dan node** `Swarm · P0 · M`
  - Terima: pesan error task tampil utuh tanpa terpotong; task lama bisa disembunyikan.
- [x] **D8S-030 — Log service dan task lewat manager** `Swarm · P0 · M`
  - Terima: log gabungan diberi awalan task dan node; bekerja untuk task di node mana pun.
- [x] **D8S-031 — Aksi service: scale, force update, rollback, hapus** `Swarm · P0 · M`
  - Terima: scale lewat dialog angka; menghapus service meminta pengguna mengetik namanya.
- [x] **D8S-029 — View stack dari label namespace, dengan hapus stack** `Swarm · P1 · M`
  - Terima: menghapus stack juga menghapus service, network, secret, dan config miliknya, seperti `docker stack rm`.
- [x] **D8S-032 — Ganti image service** `Swarm · P1 · M`
  - Terima: dialog diisi tag saat ini; update memakai spesifikasi service terbaru agar tidak menimpa perubahan lain.
- [x] **D8S-036 — Pantauan rollout live** `Swarm · P1 · M`
  - Terima: menampilkan task baru dan lama, status update, dan pesan saat rollout dijeda karena gagal.
- [x] **D8S-033 — Aksi node: drain, active, pause, promote, demote, label** `Swarm · P1 · M`
  - Terima: demote manager terakhir ditolak dengan penjelasan.
- [x] **D8S-034 — View secret dan config: daftar, pemakai, isi config, hapus** `Swarm · P1 · S`
  - Terima: nilai secret tidak pernah tampil; yang masih dipakai service tidak bisa dihapus tanpa peringatan.
- [x] **D8S-035 — Shell ke task** `Swarm · P2 · M`
  - Terima: langsung bila container ada di daemon terhubung; selain itu tampil alasan dan tawaran pindah konteks.

### Build

```bash
make build
make test lint
./bin/d8s version       # v0.6.0
make swarm-up           # 3 node docker-in-docker, context d8s-swarm (+ d8s-swarm-worker1/2), stack contoh "shop"
make test-integration   # test klien Docker terhadap cluster itu
```

`make swarm-up` butuh sekitar 30 detik. Daemon utama VM tidak ikut Swarm; cluster hidup di tiga container `d8s-swarm-*` dengan port 23750 sampai 23752 di localhost.

### Cek di VM dev

1. `./bin/d8s` di context `default`: ketik `:svc`. Tampil penjelasan bahwa engine ini bukan bagian dari swarm, bukan error mentah.
2. `:ctx`, `/d8s-swarm`, `Enter` pada `d8s-swarm`. Header menampilkan `Swarm: manager (leader)`.
3. `:no` menampilkan tiga node: satu manager (leader) dan dua worker, semuanya ready.
4. `:stk` menampilkan stack `shop` dengan `5/5`. `Enter` menampilkan dua service-nya; `Enter` pada `shop_web` menampilkan task-nya beserta node tempatnya berjalan. Breadcrumb menunjukkan `<stacks> <services> <tasks>`.
5. `Esc` ke daftar service, lalu `l` pada `shop_web`. Tiap baris log berawalan `task@node`, termasuk dari task di worker.
6. `s` pada `shop_web`: dialog terisi `3`. Ganti menjadi `5`, `Enter`. Kolom replika naik sendiri ke `5/5`.
7. **Skenario rollout gagal.** `i` pada `shop_web`, ganti image menjadi `nginx:tidak-ada`, `Enter`. Dalam belasan detik baris service berubah warna, kolom UPDATE menjadi `paused`, dan replika turun (misalnya `4/5`). `o` membuka halaman rollout: task baru yang `rejected` di atas, task lama yang masih `running` di bawah, beserta pesan jedanya.
8. `Esc`, lalu `Enter` pada `shop_web`. Task gagal tampil merah dengan pesan error lengkap (`No such image: nginx:tidak-ada`). `h` menyembunyikan task lama yang sudah berhenti bersih. `l` pada task membuka log-nya.
9. `Esc` ke daftar service, `u` pada `shop_web`: dialog menyebut image yang akan dipulihkan. Setelah `y`, replika kembali `5/5` dan UPDATE menjadi `rollback_completed`.
10. Hitung penekanan tombol dari melihat baris bermasalah sampai membaca error task: `Enter` saja; sampai log task: `Enter` lalu `l`.
11. `:no`, pilih satu worker, `a`, isi `drain`. Kolom AVAILABILITY berubah dan barisnya berganti warna; `Enter` menampilkan task node itu berpindah. Kembalikan dengan `a` dan `active`.
12. Pilih manager, `p`, `y`. Ditolak dengan pesan dari swarm bahwa manager terakhir tidak bisa di-demote.
13. Dari terminal lain `docker stop d8s-swarm-worker2`. Dalam sekitar 10 detik node itu tampil `down` dengan warna berbeda. `docker start d8s-swarm-worker2` mengembalikannya.
14. `:sec` hanya menampilkan metadata secret; `d` menampilkan inspect tanpa nilainya. `:cfg` lalu `Enter` menampilkan isi config.
15. `:tasks`, `/manager`, `s` pada salah satu task: shell terbuka di container-nya; `exit` kembali. `Esc`, `/worker1`, `s`: muncul penjelasan bahwa container ada di node lain dan tawaran pindah ke context `d8s-swarm-worker1`. Setelah `y`, header berganti dan `s` pada container itu membuka shell.
16. Kembali ke `d8s-swarm` lewat `:ctx`. `:stk`, `Ctrl-d` pada `shop`: dialog meminta namanya diketik. Nama yang salah ditolak; setelah mengetik `shop`, stack hilang dan `:svc` kosong.
17. `make swarm-down`. `docker context ls` tidak lagi memuat `d8s-swarm*`, dan `docker info` di daemon utama tetap menunjukkan Swarm tidak aktif.

**Catatan:** penawaran pindah context di langkah 15 bekerja bila ada context yang namanya sama dengan hostname node. `make swarm-up` membuatnya begitu; di cluster sungguhan, context perlu dinamai sesuai hostname node, atau pengguna pindah sendiri lewat `:ctx`.

---

## M3 · Rilis 1.0 (v1.0)

**Tujuan.** d8s aman dipakai di produksi, terkonfigurasi, teruji, dan bisa dipasang. 11 story, 23 poin.

### Story

- [x] **D8S-039 — Mode read-only lewat `--readonly` dan per konteks** `UX · P0 · S`
  - Terima: semua aksi yang mengubah keadaan ditolak di eksekutor, bukan hanya disembunyikan di UI.
- [x] **D8S-047 — Penanganan putus koneksi dan reconnect otomatis** `Koneksi · P0 · M`
  - Terima: header menandai koneksi putus; data lama diberi tanda basi; view pulih tanpa restart.
- [x] **D8S-038 — File konfigurasi: interval refresh, view awal, buffer log, shell default** `UX · P1 · S`
  - Terima: konfigurasi tidak valid menghasilkan pesan yang menunjuk barisnya; tanpa file, default berlaku.
- [x] **D8S-040 — Skin dan dukungan `NO_COLOR`** `UX · P1 · M`
  - Terima: skin terang dan gelap bawaan; semua status tetap terbaca tanpa warna.
- [x] **D8S-044 — Matriks integration test terhadap beberapa versi Docker Engine** `Kualitas · P0 · M`
  - Terima: `make test-matrix` menjalankan integration test terhadap engine 20.10 (API 1.41) dan engine terbaru, masing-masing sebagai cluster docker-in-docker tiga node di VM dev.
- [x] **D8S-046 — Uji performa dengan 2.000 container dan 500 service sintetis** `Kualitas · P1 · M`
  - Terima: `make bench` mengukur terhadap klien palsu berisi data sintetis; target di PRD terpenuhi atau selisihnya dicatat.
- [x] **D8S-043 — Pipeline rilis: build lima target, checksum, Homebrew, deb, rpm** `Kualitas · P0 · M`
  - Terima: `make release-snapshot` menghasilkan semua artefak di `dist/` tanpa mempublikasikan apa pun; `d8s version` menampilkan versi dan commit.
- [x] **D8S-045 — Dokumentasi: README, instalasi, daftar tombol, demo, panduan untuk pengguna k9s** `Kualitas · P0 · S`
  - Terima: pengguna baru bisa memasang dan membuka log container hanya dari README.
- [x] **D8S-041 — Alias dan hotkey kustom** `UX · P2 · S`
  - Terima: hotkey kustom yang bentrok dengan bawaan dilaporkan saat startup.
- [x] **D8S-042 — Kolom kustom dan sort tersimpan per view** `UX · P2 · M`
  - Terima: pilihan kolom dan sort bertahan antarsesi.
- [x] **D8S-048 — Log debug lewat `--log-file` dan perintah `d8s info`** `Kualitas · P2 · S`
  - Terima: `d8s info` mencetak versi, konteks, versi API, dan lokasi konfigurasi.

### Build

```bash
make tools              # menambah goreleaser ke bin/tools (±1 menit)
make build
make test lint
./bin/d8s version       # v1.0.0
make bench              # ±10 detik
make test-matrix        # ±2 menit; membangun ulang cluster uji untuk tiap versi engine, lalu menghapusnya
make release-snapshot   # ±20 detik; mengisi dist/
```

### Cek di VM dev

1. `ls dist/` memuat arsip untuk linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 (`.tar.gz`) dan windows/amd64 (`.zip`), `checksums.txt`, dua `.deb`, dua `.rpm`, dan `homebrew/Casks/d8s.rb`.
2. Pasang paketnya di container bersih, tanpa mengubah VM:
   ```bash
   docker run --rm -v $PWD/dist:/dist public.ecr.aws/docker/library/debian:stable-slim \
     sh -c 'dpkg -i /dist/d8s_1.0.0_linux_amd64.deb && d8s version'
   ```
   Keluarannya `d8s v1.0.0 (…)`.
3. `make test-matrix` berakhir dengan tabel dua baris `pass`: engine 20.10 (API 1.41) dan engine terbaru.
4. `make bench` mencetak enam ukuran, semuanya `pass`.
5. `make demo-up`, lalu `./bin/d8s --readonly`. Header menampilkan `Mode: READ-ONLY`. `r`, `x`, dan `Ctrl-d` pada `d8s-demo-web` ditolak dengan pesan dan tanpa dialog; `s` (shell) juga ditolak; `l`, `d`, dan `h` tetap berfungsi. Container tetap ada.
6. Buat `~/.config/d8s/config.yaml` berisi `defaultView: images`; d8s langsung terbuka di view image. Tambahkan baris `refesh: 5s` (salah ketik); d8s menolak jalan dan menyebut `line 2` beserta barisnya. `d8s info` melaporkan hal yang sama. Hapus baris itu.
7. Tambahkan `skin: light`; warna berganti dan tetap terbaca di terminal berlatar terang. `NO_COLOR=1 ./bin/d8s` tampil tanpa warna: baris terpilih terbalik, container `exited` redup.
8. Tambahkan ke konfigurasi:
   ```yaml
   aliases:
     ng: images /nginx
     c: images
   hotkeys:
     f2: ng
     r: images
   ```
   Saat mulai, baris status melaporkan bahwa alias `c` dan hotkey `r` bentrok dan diabaikan. `F2` membuka image tersaring `nginx`. `?` mencantumkan `<:ng>` di COMMANDS dan `<f2>` di HOTKEYS.
9. Tambahkan `views: {containers: {columns: [NAME, STATE, CPU%, MEM]}}`; `:c` hanya menampilkan empat kolom itu. `Shift-m` dua kali, `:q`, buka lagi: `:c` masih terurut menurut `MEM↓`.
10. `./bin/d8s info` mencetak versi, lokasi dan status konfigurasi, lokasi state, context, host, engine, API, dan peran Swarm. `./bin/d8s --log-file /tmp/d8s.log`, lakukan satu aksi, keluar; berkasnya memuat baris `started` dan `action`.
11. **Reconnect.** `make swarm-up`, `./bin/d8s --context d8s-swarm`, `:svc`. Dari terminal lain `docker restart d8s-swarm-manager` (tanpa `-t`). Dalam dua detik header menampilkan `Daemon: DISCONNECTED` dan judul tabel `(stale)`, baris lama tetap terlihat. Sekitar 25 detik kemudian keduanya hilang sendiri dan tabel memuat keadaan baru. Daemon utama VM tidak di-restart.
12. Tambahkan `contexts: {d8s-swarm: {readOnly: true, production: true}}`. Di context `default` semua aksi jalan; setelah `:ctx` ke `d8s-swarm`, header menampilkan `d8s-swarm (production)` dan `READ-ONLY`, dan scale ditolak.
13. Ulangi cek M1 dan M2 dengan binary dari `dist/d8s_linux_amd64_v1/d8s`.
14. Ikuti `README.md` dari "Pasang" sampai "Lima menit pertama" tanpa bantuan lain.
15. `make demo-down && make swarm-down`, dan hapus `~/.config/d8s/config.yaml` bila tidak ingin dipakai terus.

**Tidak bisa dicek di VM ini:**

- Binary macOS dan Windows hanya terbukti ter-build. Shell interaktif butuh `/dev/tty`, jadi di Windows belum berfungsi; README menyebutnya.
- Cask Homebrew hanya terbukti ter-generate.
- Publikasi ke GitHub Releases butuh token dan dijalankan di luar VM.
- Paket `.rpm` bisa dicek seperti `.deb` dengan image `public.ecr.aws/docker/library/fedora`, bila diinginkan.

**Catatan:** `make test-matrix` dan `make swarm-up` menarik image dari mirror publik (`public.ecr.aws`), karena Docker Hub membatasi tarikan anonim. Me-restart container docker-in-docker dengan paksa (`docker restart -t 1`) bisa membuat daemon di dalamnya tidak hidup lagi; itu sifat cluster uji, bukan d8s. `make swarm-up` membangunnya ulang.

---

## Pasca-1.0

Ide berikut sengaja tidak diestimasi dan tidak masuk hitungan di atas.

- Deploy dan update stack dari Compose file.
- Plugin dan perintah eksternal, seperti di k9s.
- Ringkasan cluster satu layar (padanan `:pulse`).
- Tampilan pohon relasi stack, service, task, dan node (padanan XRay).
- Dukungan Podman.
- Stats lintas node lewat koneksi ke beberapa konteks sekaligus.
