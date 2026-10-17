# app_dir <app>: apps live in testapps/<app> or, for the benchmark suite, testapps/suite/<app>
app_dir() {
  if [ -d "$REPO/testapps/$1" ]; then echo "$REPO/testapps/$1"; else echo "$REPO/testapps/suite/$1"; fi
}

# all_apps: core apps, extra apps, then the suite
all_apps() {
  echo $CORE_APPS $EXTRA_APPS $(ls "$REPO/testapps/suite" 2>/dev/null)
}
