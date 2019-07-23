import requests
from bs4 import BeautifulSoup
import pandas as pd

# make HTTPS GET request to HN
request = requests.get("https://news.ycombinator.com/")

# parse HTML with bs4
soup = BeautifulSoup(request.content, features="html.parser")

# select posts from table
posts = soup.select('table.itemlist > tr')

# messy bit: extract relevant information from HTML
post_titles = []
post_authors = []
post_urls = []

for i, post in enumerate(posts):
    
    if post.get('class') and 'athing' in post.get('class'):
        post_titles.append(
            post.select('a.storylink')[0].text)
        post_urls.append(
            post.select('a.storylink')[0].get('href'))
        
        user_el = posts[i+1].select('a.hnuser')
        if len(user_el) > 0:
            post_authors.append(user_el[0].text)
        else:
            post_authors.append('No author')
        

# construct dataframe from lists
df = pd.DataFrame({
    "titles": post_titles, 
    "authors": post_authors, 
    "urls": post_urls})

# write dataframe to sheet starting at A1 position
sheet("A1", df, headers=True)
