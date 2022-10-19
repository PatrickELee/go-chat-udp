for CMD in `ls cmd`; do
  trimmed=${CMD%.bar}".exe"
  go build -o $trimmed ./cmd/$CMD
  echo $trimmed
done
./server