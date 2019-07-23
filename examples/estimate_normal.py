import numpy as np
import pandas as pd
import math
import time

n = 10000

for i in range(9):
    
    sample_n = 2**i * 40
    
    print(sample_n)

    normally_dist = np.random.randn(sample_n)
    rep_count = math.ceil(n / sample_n)
    normally_dist = np.repeat(normally_dist, rep_count)
    
    # limit
    normally_dist = normally_dist[0:n]
    
    sheet("A1", pd.DataFrame(normally_dist))
    
    time.sleep(1)