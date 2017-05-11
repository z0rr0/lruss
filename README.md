# LRUSS

LRUSS is a Redis based URL Shortening Service.

It's a simple web app to convert incoming URL address 
to short one using custom domain. 

[Redis](https://redis.io/) is used as a storage and cache.

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
