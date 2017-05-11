# LRUSS

LRUSS is a Redis based URL Shortening Service.

## Build

Run to build:

```bash
make install
```

to local start:

```bash
make run
```

to test:

```bash
make test
```

## Administration

Create use "admin" and get a password:

```bash
./lruss -adminpass admin -config config.json
```

Open URL `/admin/login`.
