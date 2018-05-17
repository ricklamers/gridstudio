git push origin master

ssh rick@beta.gridstudio.io -p 1893 << EOF
  cd /home/rick/workspace/grid-docker
  git reset --hard
  git pull
EOF

echo "Updated remote server."