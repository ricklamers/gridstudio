import time
import random

def fillRandom():
    print(random.random())
    
fillRandom()

x = 5

while x > 0:
    fillRandom()
    time.sleep(1)
    x -= 1