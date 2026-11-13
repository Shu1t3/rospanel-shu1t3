from http.server import BaseHTTPRequestHandler, HTTPServer
from socket import AF_INET6, SOCK_DGRAM, socket
from threading import Thread
class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        body = self.client_address[0].encode()
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
class Server(HTTPServer):
    address_family=AF_INET6
def udp_echo():
    with socket(AF_INET6, SOCK_DGRAM) as udp:
        udp.bind(('2600:1000::50', 18081))
        while True:
            payload, addr = udp.recvfrom(1024)
            udp.sendto(payload+b'@'+addr[0].encode(),addr)
Thread(target=udp_echo,daemon=True).start()
Server(('2600:1000::50',18080),Handler).serve_forever()
